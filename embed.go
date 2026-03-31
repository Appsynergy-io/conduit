package conduit

import "embed"

// FrontendFS holds the static Next.js export built from web/.
// The directory is created by `cd web && pnpm build` which writes to web/out/.
// During development before the frontend is built, this embed will be empty.
//
//go:embed all:web/out
var FrontendFS embed.FS
