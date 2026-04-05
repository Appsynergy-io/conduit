// Proof-of-work CAPTCHA client solver.
//
// Fetches a challenge from /api/v1/auth/captcha/challenge, runs the PoW
// solve loop in a Web Worker to keep the UI responsive, and returns a
// Solution object ready to embed in the recovery request body.
//
// Uses a pure-JS SHA-256 implementation (no dependencies, no external code)
// in a Worker spawned from an inline Blob. The Worker reports progress and
// terminates once a valid nonce is found.

export interface CaptchaChallenge {
  salt: string
  target: string
  expires: number
  endpoint: string
  hmac: string
}

export interface CaptchaSolution extends CaptchaChallenge {
  nonce: number
}

export interface SolveProgress {
  iterations: number
  elapsedMs: number
}

const WORKER_SOURCE = `
// SHA-256 constants (FIPS 180-4)
const K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
  0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
  0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
  0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
  0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
  0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
  0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
])

function rotr(x, n) { return ((x >>> n) | (x << (32 - n))) >>> 0 }

// sha256 returns a 32-byte Uint8Array. Input length is always 40 for our
// payload (32-byte salt + 8-byte big-endian nonce). 40 bytes + 1 padding
// byte + 15 zero bytes + 8 length bytes = 64 bytes = ONE block.
function sha256of40(salt, nonceHi, nonceLo) {
  const W = new Uint32Array(64)
  // Hash state (FIPS 180-4 initial values)
  let h0 = 0x6a09e667, h1 = 0xbb67ae85, h2 = 0x3c6ef372, h3 = 0xa54ff53a
  let h4 = 0x510e527f, h5 = 0x9b05688c, h6 = 0x1f83d9ab, h7 = 0x5be0cd19

  // Single 64-byte block layout:
  //   bytes  0..31 : salt                → W[0..7]
  //   bytes 32..39 : nonce BE uint64     → W[8], W[9]
  //   byte     40  : 0x80 pad            → W[10] = 0x80000000
  //   bytes 41..55 : zeros               → W[10].rest, W[11], W[12], W[13]
  //   bytes 56..63 : length BE uint64 (320 bits = 40*8)
  //                  W[14] = 0, W[15] = 320
  for (let i = 0; i < 8; i++) {
    W[i] = (salt[i*4]<<24 | salt[i*4+1]<<16 | salt[i*4+2]<<8 | salt[i*4+3]) >>> 0
  }
  W[8] = nonceHi >>> 0
  W[9] = nonceLo >>> 0
  W[10] = 0x80000000
  W[11] = 0; W[12] = 0; W[13] = 0; W[14] = 0
  W[15] = 320

  processBlock()

  const out = new Uint8Array(32)
  const hs = [h0, h1, h2, h3, h4, h5, h6, h7]
  for (let i = 0; i < 8; i++) {
    out[i*4]   = (hs[i] >>> 24) & 0xff
    out[i*4+1] = (hs[i] >>> 16) & 0xff
    out[i*4+2] = (hs[i] >>>  8) & 0xff
    out[i*4+3] = hs[i] & 0xff
  }
  return out

  function processBlock() {
    for (let t = 16; t < 64; t++) {
      const s0 = rotr(W[t-15], 7) ^ rotr(W[t-15], 18) ^ (W[t-15] >>> 3)
      const s1 = rotr(W[t-2], 17) ^ rotr(W[t-2], 19) ^ (W[t-2] >>> 10)
      W[t] = (W[t-16] + s0 + W[t-7] + s1) >>> 0
    }
    let a = h0, b = h1, c = h2, d = h3, e = h4, f = h5, g = h6, h = h7
    for (let t = 0; t < 64; t++) {
      const S1 = rotr(e, 6) ^ rotr(e, 11) ^ rotr(e, 25)
      const ch = (e & f) ^ (~e & g)
      const t1 = (h + S1 + ch + K[t] + W[t]) >>> 0
      const S0 = rotr(a, 2) ^ rotr(a, 13) ^ rotr(a, 22)
      const mj = (a & b) ^ (a & c) ^ (b & c)
      const t2 = (S0 + mj) >>> 0
      h = g; g = f; f = e; e = (d + t1) >>> 0
      d = c; c = b; b = a; a = (t1 + t2) >>> 0
    }
    h0 = (h0 + a) >>> 0; h1 = (h1 + b) >>> 0; h2 = (h2 + c) >>> 0; h3 = (h3 + d) >>> 0
    h4 = (h4 + e) >>> 0; h5 = (h5 + f) >>> 0; h6 = (h6 + g) >>> 0; h7 = (h7 + h) >>> 0
  }
}

function lessThan(a, target) {
  for (let i = 0; i < 32; i++) {
    if (a[i] < target[i]) return true
    if (a[i] > target[i]) return false
  }
  return false
}

function hexToBytes(hex) {
  const out = new Uint8Array(hex.length / 2)
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(hex.substr(i*2, 2), 16)
  }
  return out
}

self.onmessage = function(e) {
  const { salt: saltHex, target: targetHex, maxIterations } = e.data
  const salt = hexToBytes(saltHex)
  const target = hexToBytes(targetHex)
  const start = performance.now()
  const max = maxIterations || 20000000
  const reportEvery = 65536

  for (let nonce = 0; nonce < max; nonce++) {
    const hi = Math.floor(nonce / 0x100000000)
    const lo = nonce >>> 0
    const hash = sha256of40(salt, hi, lo)
    if (lessThan(hash, target)) {
      self.postMessage({ type: 'done', nonce, iterations: nonce + 1, elapsedMs: performance.now() - start })
      return
    }
    if ((nonce & (reportEvery - 1)) === reportEvery - 1) {
      self.postMessage({ type: 'progress', iterations: nonce + 1, elapsedMs: performance.now() - start })
    }
  }
  self.postMessage({ type: 'exhausted' })
}
`

/**
 * Fetches a fresh CAPTCHA challenge for the given endpoint from the server.
 */
export async function fetchChallenge(endpoint: string): Promise<CaptchaChallenge> {
  const res = await fetch(`/api/v1/auth/captcha/challenge?endpoint=${encodeURIComponent(endpoint)}`)
  if (!res.ok) {
    throw new Error(`captcha challenge failed: ${res.status}`)
  }
  return res.json()
}

/**
 * Solves a CAPTCHA challenge using a Web Worker. Returns the completed
 * Solution including the found nonce.
 *
 * @param challenge  The challenge issued by the server.
 * @param onProgress Optional callback invoked periodically with iteration count.
 */
export function solveChallenge(
  challenge: CaptchaChallenge,
  onProgress?: (p: SolveProgress) => void,
): Promise<CaptchaSolution> {
  return new Promise((resolve, reject) => {
    const blob = new Blob([WORKER_SOURCE], { type: "application/javascript" })
    const url = URL.createObjectURL(blob)
    const worker = new Worker(url)

    const cleanup = () => {
      worker.terminate()
      URL.revokeObjectURL(url)
    }

    worker.onmessage = (e: MessageEvent) => {
      const msg = e.data
      if (msg.type === "progress") {
        onProgress?.({ iterations: msg.iterations, elapsedMs: msg.elapsedMs })
      } else if (msg.type === "done") {
        cleanup()
        resolve({ ...challenge, nonce: msg.nonce })
      } else if (msg.type === "exhausted") {
        cleanup()
        reject(new Error("captcha: iteration cap reached"))
      }
    }
    worker.onerror = (err) => {
      cleanup()
      reject(err)
    }
    worker.postMessage({
      salt: challenge.salt,
      target: challenge.target,
    })
  })
}

/**
 * Convenience wrapper: fetch a challenge and solve it.
 */
export async function solveCaptcha(
  endpoint: string,
  onProgress?: (p: SolveProgress) => void,
): Promise<CaptchaSolution> {
  const challenge = await fetchChallenge(endpoint)
  return solveChallenge(challenge, onProgress)
}
