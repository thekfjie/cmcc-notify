import assert from "node:assert/strict"
import test from "node:test"

import { extractCMCCAPIKey, normalizeCMCCAPIKey } from "./api-key.ts"

const key = "ak_01234567-89ab-cdef-0123-456789abcdef"

test("extracts a bare Channel API Key", () => {
  assert.equal(extractCMCCAPIKey(key), key)
  assert.equal(normalizeCMCCAPIKey(`  ${key}\n`), key)
})

test("extracts a Channel API Key from the complete authorization message", () => {
  const message = `【新消息Claw】请根据 https://example.invalid/channel-guide.md 安装中国移动新消息Channel插件。我的新消息Channel API Key为${key}`
  assert.equal(extractCMCCAPIKey(message), key)
  assert.equal(normalizeCMCCAPIKey(message), key)
})

test("keeps invalid text available for the existing validation path", () => {
  const invalid = "授权成功，但这段消息里没有 API Key"
  assert.equal(extractCMCCAPIKey(invalid), null)
  assert.equal(normalizeCMCCAPIKey(`  ${invalid}  `), invalid)
  assert.equal(extractCMCCAPIKey("ak_"), null)
})
