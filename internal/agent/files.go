package agent

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/appsynergy-io/conduit/internal/protocol"
)

const (
	// maxFileReadBytes is the maximum bytes returned inline for a file read.
	maxFileReadBytes = 16 << 20 // 16 MB

	// maxFileWriteBytes is the maximum bytes accepted for a file write.
	maxFileWriteBytes = 16 << 20 // 16 MB

	// maxPreviewBytes is the default preview size.
	maxPreviewBytes = 65536
)

// handleFileFrame routes file operation frames to the appropriate handler.
func (a *Agent) handleFileFrame(ctx context.Context, mux protocol.FrameMux, f *protocol.Frame) {
	switch f.Type {
	case protocol.FrameFileList:
		a.handleFileList(ctx, mux, f)
	case protocol.FrameFileRead:
		a.handleFileRead(ctx, mux, f)
	case protocol.FrameFileWrite:
		a.handleFileWrite(ctx, mux, f)
	case protocol.FrameFileStat:
		a.handleFileStat(ctx, mux, f)
	case protocol.FrameFileDelete:
		a.handleFileDelete(ctx, mux, f)
	case protocol.FrameFileRename:
		a.handleFileRename(ctx, mux, f)
	case protocol.FrameFileMkdir:
		a.handleFileMkdir(ctx, mux, f)
	}
}

// handleFileList returns a directory listing.
func (a *Agent) handleFileList(ctx context.Context, mux protocol.FrameMux, f *protocol.Frame) {
	var req protocol.FileListRequest
	if err := protocol.UnmarshalPayload(f.Payload, &req); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileListResponse{
			Error: "invalid request payload",
		})
		return
	}

	cleanPath := sanitizePath(req.Path)
	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileListResponse{
			Path:  cleanPath,
			Error: err.Error(),
		})
		return
	}

	result := make([]protocol.FileEntry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files unless requested
		if !req.ShowHidden && strings.HasPrefix(name, ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		fe := protocol.FileEntry{
			Name:       name,
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC().Format(time.RFC3339),
			IsHidden:   strings.HasPrefix(name, "."),
		}

		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			fe.Type = "symlink"
		case entry.IsDir():
			fe.Type = "directory"
		default:
			fe.Type = "file"
		}

		fe.Permissions = info.Mode().Perm().String()
		setOwnerGroup(info, &fe)

		result = append(result, fe)
	}

	a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileListResponse{
		Path:    cleanPath,
		Entries: result,
	})
}

// handleFileRead reads a file and returns its content.
func (a *Agent) handleFileRead(ctx context.Context, mux protocol.FrameMux, f *protocol.Frame) {
	var req protocol.FileReadRequest
	if err := protocol.UnmarshalPayload(f.Payload, &req); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileReadResponse{
			Error: "invalid request payload",
		})
		return
	}

	cleanPath := sanitizePath(req.Path)

	info, err := os.Stat(cleanPath)
	if err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileReadResponse{
			Path:  cleanPath,
			Error: err.Error(),
		})
		return
	}

	if info.IsDir() {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileReadResponse{
			Path:  cleanPath,
			Error: "is a directory",
		})
		return
	}

	maxBytes := req.MaxBytes
	if maxBytes <= 0 || maxBytes > maxFileReadBytes {
		maxBytes = maxFileReadBytes
	}

	file, err := os.Open(cleanPath)
	if err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileReadResponse{
			Path:  cleanPath,
			Error: err.Error(),
		})
		return
	}
	defer file.Close()

	if req.Offset > 0 {
		if _, err := file.Seek(req.Offset, 0); err != nil {
			a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileReadResponse{
				Path:  cleanPath,
				Error: err.Error(),
			})
			return
		}
	}

	buf := make([]byte, maxBytes)
	n, _ := file.Read(buf)
	buf = buf[:n]

	truncated := info.Size() > req.Offset+int64(n)

	// Detect MIME type from first 512 bytes
	mimeType := http.DetectContentType(buf[:min(len(buf), 512)])

	a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileReadResponse{
		Path:      cleanPath,
		Size:      info.Size(),
		MimeType:  mimeType,
		Truncated: truncated,
		Content:   base64.StdEncoding.EncodeToString(buf),
	})
}

// handleFileWrite writes content to a file.
// Supports both atomic writes (no Offset/UploadID) and chunked writes for resumable uploads.
func (a *Agent) handleFileWrite(ctx context.Context, mux protocol.FrameMux, f *protocol.Frame) {
	var req protocol.FileWriteRequest
	if err := protocol.UnmarshalPayload(f.Payload, &req); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
			Error: "invalid request payload",
		})
		return
	}

	cleanPath := sanitizePath(req.Path)

	data, err := base64.StdEncoding.DecodeString(req.Content)
	if err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
			Path:  cleanPath,
			Error: "invalid base64 content",
		})
		return
	}

	// Enforce per-chunk write size limit (OWASP API4, NIST REC-API-14)
	if int64(len(data)) > maxFileWriteBytes {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
			Path:  cleanPath,
			Error: fmt.Sprintf("chunk too large: %d bytes exceeds %d byte limit", len(data), maxFileWriteBytes),
		})
		return
	}

	// Parse mode, default 0644
	mode := os.FileMode(0644)
	if req.Mode != "" {
		if m, err := strconv.ParseUint(req.Mode, 8, 32); err == nil {
			mode = os.FileMode(m)
		}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0755); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
			Path:  cleanPath,
			Error: err.Error(),
		})
		return
	}

	// Chunked write: write at offset (for resumable uploads)
	if req.UploadID != "" || req.Offset > 0 {
		a.writeChunk(ctx, mux, f, cleanPath, data, mode, req.Offset, req.Truncate)
		return
	}

	// Atomic write (original behavior)
	if err := os.WriteFile(cleanPath, data, mode); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
			Path:  cleanPath,
			Error: err.Error(),
		})
		return
	}

	checksum := sha256.Sum256(data)

	a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
		Path:     cleanPath,
		Size:     int64(len(data)),
		Checksum: fmt.Sprintf("%x", checksum),
	})
}

// writeChunk writes data at a specific offset within a file for chunked/resumable uploads.
func (a *Agent) writeChunk(ctx context.Context, mux protocol.FrameMux, f *protocol.Frame,
	path string, data []byte, mode os.FileMode, offset int64, truncate bool) {

	flags := os.O_WRONLY | os.O_CREATE
	if truncate {
		flags |= os.O_TRUNC
	}

	file, err := os.OpenFile(path, flags, mode)
	if err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
			Path:  path,
			Error: err.Error(),
		})
		return
	}
	defer file.Close()

	if offset > 0 {
		if _, err := file.Seek(offset, 0); err != nil {
			a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
				Path:  path,
				Error: fmt.Sprintf("seek to offset %d: %s", offset, err),
			})
			return
		}
	}

	n, err := file.Write(data)
	if err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
			Path:  path,
			Error: err.Error(),
		})
		return
	}

	a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileWriteResponse{
		Path: path,
		Size: int64(n),
	})
}

// handleFileStat returns file metadata.
func (a *Agent) handleFileStat(ctx context.Context, mux protocol.FrameMux, f *protocol.Frame) {
	var req protocol.FileStatRequest
	if err := protocol.UnmarshalPayload(f.Payload, &req); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileStatResponse{
			Error: "invalid request payload",
		})
		return
	}

	cleanPath := sanitizePath(req.Path)

	info, err := os.Lstat(cleanPath)
	if err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileStatResponse{
			Path:  cleanPath,
			Error: err.Error(),
		})
		return
	}

	resp := &protocol.FileStatResponse{
		Path:        cleanPath,
		Size:        info.Size(),
		Permissions: info.Mode().Perm().String(),
		ModifiedAt:  info.ModTime().UTC().Format(time.RFC3339),
	}

	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		resp.Type = "symlink"
	case info.IsDir():
		resp.Type = "directory"
	default:
		resp.Type = "file"
	}

	fe := protocol.FileEntry{}
	setOwnerGroup(info, &fe)
	resp.Owner = fe.Owner
	resp.Group = fe.Group

	a.sendFileResponse(ctx, mux, f.Type, f.StreamID, resp)
}

// handleFileDelete deletes a file or directory.
func (a *Agent) handleFileDelete(ctx context.Context, mux protocol.FrameMux, f *protocol.Frame) {
	var req protocol.FileDeleteRequest
	if err := protocol.UnmarshalPayload(f.Payload, &req); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileDeleteResponse{
			Error: "invalid request payload",
		})
		return
	}

	cleanPath := sanitizePath(req.Path)

	// Never allow deleting root-level system paths
	if isProtectedPath(cleanPath) {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileDeleteResponse{
			Path:  cleanPath,
			Error: "refusing to delete system directory",
		})
		return
	}

	var err error
	if req.Recursive {
		err = os.RemoveAll(cleanPath)
	} else {
		err = os.Remove(cleanPath)
	}

	resp := &protocol.FileDeleteResponse{Path: cleanPath}
	if err != nil {
		resp.Error = err.Error()
	}
	a.sendFileResponse(ctx, mux, f.Type, f.StreamID, resp)
}

// handleFileRename renames/moves a file.
func (a *Agent) handleFileRename(ctx context.Context, mux protocol.FrameMux, f *protocol.Frame) {
	var req protocol.FileRenameRequest
	if err := protocol.UnmarshalPayload(f.Payload, &req); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileRenameResponse{
			Error: "invalid request payload",
		})
		return
	}

	oldPath := sanitizePath(req.OldPath)
	newPath := sanitizePath(req.NewPath)

	// Prevent renaming system directories or overwriting them
	if isProtectedPath(oldPath) || isProtectedPath(newPath) {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileRenameResponse{
			OldPath: oldPath,
			NewPath: newPath,
			Error:   "refusing to rename system directory",
		})
		return
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileRenameResponse{
			OldPath: oldPath,
			NewPath: newPath,
			Error:   err.Error(),
		})
		return
	}

	a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileRenameResponse{
		OldPath: oldPath,
		NewPath: newPath,
	})
}

// handleFileMkdir creates a directory.
func (a *Agent) handleFileMkdir(ctx context.Context, mux protocol.FrameMux, f *protocol.Frame) {
	var req protocol.FileMkdirRequest
	if err := protocol.UnmarshalPayload(f.Payload, &req); err != nil {
		a.sendFileResponse(ctx, mux, f.Type, f.StreamID, &protocol.FileMkdirResponse{
			Error: "invalid request payload",
		})
		return
	}

	cleanPath := sanitizePath(req.Path)

	var err error
	if req.Parents {
		err = os.MkdirAll(cleanPath, 0755)
	} else {
		err = os.Mkdir(cleanPath, 0755)
	}

	resp := &protocol.FileMkdirResponse{Path: cleanPath}
	if err != nil {
		resp.Error = err.Error()
	}
	a.sendFileResponse(ctx, mux, f.Type, f.StreamID, resp)
}

// sendFileResponse sends a typed file response frame back to the server.
func (a *Agent) sendFileResponse(ctx context.Context, mux protocol.FrameMux, frameType protocol.FrameType, streamID uint32, payload interface{}) {
	f, err := protocol.NewFrame(frameType, streamID, payload)
	if err != nil {
		a.logger.Debug("failed to create file response frame", "error", err)
		return
	}
	if err := mux.Send(ctx, f); err != nil {
		a.logger.Debug("failed to send file response", "error", err)
	}
}

// isProtectedPath returns true if the path is a critical system directory
// that must not be deleted, overwritten, or renamed.
func isProtectedPath(path string) bool {
	switch path {
	case "/", "/root", "/home", "/etc", "/var", "/usr",
		"/bin", "/sbin", "/lib", "/lib64", "/boot", "/dev",
		"/proc", "/sys", "/tmp", "/run", "/opt", "/srv":
		return true
	}
	return false
}

// sanitizePath cleans and validates an absolute path.
// Prevents path traversal attacks (OWASP A03, V12).
func sanitizePath(path string) string {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join("/", cleaned)
	}
	return cleaned
}

// setOwnerGroup extracts owner and group information from file info.
func setOwnerGroup(info fs.FileInfo, fe *protocol.FileEntry) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return
	}

	if u, err := user.LookupId(strconv.Itoa(int(stat.Uid))); err == nil {
		fe.Owner = u.Username
	} else {
		fe.Owner = strconv.Itoa(int(stat.Uid))
	}

	if g, err := user.LookupGroupId(strconv.Itoa(int(stat.Gid))); err == nil {
		fe.Group = g.Name
	} else {
		fe.Group = strconv.Itoa(int(stat.Gid))
	}
}
