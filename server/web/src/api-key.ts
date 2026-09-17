const apiKeyPattern = /(?:^|[^A-Za-z0-9_])((?:ak|app)_[A-Za-z0-9][A-Za-z0-9_-]*)/

export function extractCMCCAPIKey(value: string): string | null {
  return apiKeyPattern.exec(value.trim())?.[1] ?? null
}

export function normalizeCMCCAPIKey(value: string): string {
  return extractCMCCAPIKey(value) ?? value.trim()
}
