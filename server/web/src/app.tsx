import { type FormEvent, type ReactNode, useCallback, useEffect, useMemo, useState } from "react"
import {
  Activity, AlertTriangle, BellRing, BookOpen, Check, CheckCircle2, Clock3, Copy, Eye, EyeOff,
  ExternalLink, FileText, FileUp, Image as ImageIcon, KeyRound, Link2, LoaderCircle, LockKeyhole,
  LogOut, MessageSquareText, Moon, Network, Pencil, Plus, Radio, RefreshCw, RotateCcw, Save, Send,
  QrCode, Server as ServerIcon, ShieldCheck, Smartphone, Sun, Trash2, UsersRound, Wifi,
  WifiOff, X,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput, InputGroupText, InputGroupTextarea } from "@/components/ui/input-group"
import { Select } from "@/components/ui/select"
import { cn } from "@/lib/utils"

type Page = "overview" | "send" | "accounts" | "docs"
type Theme = "light" | "dark"
type MediaType = "AUTO" | "IMAGE" | "AUDIO" | "VIDEO" | "FILE"

type AccountStatus = { name: string; enabled: boolean; connected: boolean; api_key: string; default_to?: string }
type AccountSetting = AccountStatus & { has_api_key: boolean; server_url?: string; upload_url?: string }
type RecipientGroup = { name: string; recipients: string[] }
type ApplicationSetting = {
  name: string; enabled: boolean; account: string; to?: string; group?: string; token_hint: string
  allow_recipient_override: boolean; rate_limit_per_minute: number
}
type StatusResponse = { status: string; version: string; uptime_seconds: number; accounts: AccountStatus[] }
type SettingsResponse = { writable: boolean; accounts: AccountSetting[]; groups: RecipientGroup[]; applications: ApplicationSetting[]; application_token?: string }
type RecipientSendResult = { to: string; message_id?: string; accepted?: boolean; acknowledged?: boolean; error?: string }
type SendResponse = {
  message_id?: string; accepted?: boolean; acknowledged?: boolean; group?: string; total?: number
  accepted_count?: number; failed_count?: number; media_type?: string; file_name?: string; size?: number
  mode?: "local_fanout"; native_broadcast?: boolean; results?: RecipientSendResult[]
}
type APIErrorBody = { error?: string }

class APIError extends Error {
  status: number
  constructor(status: number, message: string) { super(message); this.status = status }
}

const sessionTokenKey = "cmcc-notify-token"
const themeKey = "cmcc-notify-theme"
const activationURL = "https://rcs.10086.cn/i/#/?RPwXWk9k0yk"
const navigation: Array<{ id: Page; label: string; icon: typeof Activity }> = [
  { id: "overview", label: "概览", icon: Activity },
  { id: "send", label: "发送消息", icon: Send },
  { id: "accounts", label: "账户状态", icon: UsersRound },
  { id: "docs", label: "API 文档", icon: BookOpen },
]

async function apiRequest<T>(path: string, token: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  headers.set("Accept", "application/json")
  headers.set("Authorization", `Bearer ${token}`)
  if (init?.body && !(init.body instanceof FormData)) headers.set("Content-Type", "application/json")
  const response = await fetch(path, { ...init, headers })
  let body: T | APIErrorBody | undefined
  if (response.status !== 204) {
    try { body = (await response.json()) as T | APIErrorBody } catch { body = undefined }
  }
  if (!response.ok) {
    const message = body && typeof body === "object" && "error" in body && body.error ? body.error : `请求失败（HTTP ${response.status}）`
    throw new APIError(response.status, message)
  }
  return body as T
}

function preferredTheme(): Theme {
  const stored = localStorage.getItem(themeKey)
  if (stored === "light" || stored === "dark") return stored
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
}

function formatUptime(totalSeconds: number) {
  const seconds = Math.max(0, Math.floor(totalSeconds)); const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600); const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days} 天 ${hours} 小时`; if (hours > 0) return `${hours} 小时 ${minutes} 分`
  if (minutes > 0) return `${minutes} 分 ${seconds % 60} 秒`; return `${seconds} 秒`
}
function formatVersion(version: string) { if (!version || version === "dev") return "dev"; return version.startsWith("v") ? version : `v${version}` }
function formatBytes(bytes?: number) {
  if (!bytes) return "0 B"; const units = ["B", "KB", "MB", "GB"]
  const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / 1024 ** index).toFixed(index ? 1 : 0)} ${units[index]}`
}
function humanizeError(error: unknown) {
  if (!(error instanceof Error)) return "发生未知错误，请稍后重试。"
  const translations: Record<string, string> = {
    unauthorized: "访问令牌无效，请检查后重试。", "invalid JSON": "请求内容格式无效。",
    "account already exists": "这个通道名称已经存在，请换一个名称。", "account not found": "要编辑的通道不存在，请刷新后重试。",
    "unknown or disabled account": "通道不存在或已被停用。", "unknown group": "本地号码组不存在。",
    "media.url is required": "多媒体消息必须填写文件 URL。", "media.url must use http or https": "文件 URL 必须使用 HTTP 或 HTTPS。",
    "text is required": "消息内容不能为空。", "recipient is required": "请填写 to 目标，或为通道配置默认目标。",
    "file is required": "请选择要上传的文件。", "application is disabled": "通知应用已停用。",
    "application rate limit exceeded": "通知应用已达到每分钟请求上限，请稍后重试。", "recipient override is not allowed": "该应用不允许临时修改发送目标。",
  }
  return translations[error.message] ?? error.message
}

export function App() {
  const [theme, setTheme] = useState<Theme>(preferredTheme)
  const [token, setToken] = useState(() => sessionStorage.getItem(sessionTokenKey) ?? "")
  const [status, setStatus] = useState<StatusResponse | null>(null)
  const [checking, setChecking] = useState(Boolean(token)); const [loginError, setLoginError] = useState("")
  useEffect(() => { document.documentElement.classList.toggle("dark", theme === "dark"); document.documentElement.style.colorScheme = theme; localStorage.setItem(themeKey, theme) }, [theme])
  useEffect(() => {
    if (!token || status) return; let active = true
    apiRequest<StatusResponse>("/v1/status", token).then((response) => { if (active) setStatus(response) }).catch((error: unknown) => {
      if (!active) return; sessionStorage.removeItem(sessionTokenKey); setToken(""); setLoginError(humanizeError(error))
    }).finally(() => { if (active) setChecking(false) })
    return () => { active = false }
  }, [status, token])
  const toggleTheme = () => setTheme((current) => current === "light" ? "dark" : "light")
  if (checking) return <div className="auth-stage"><div className="loading-mark" aria-label="正在验证登录状态"><BrandMark /><LoaderCircle className="size-5 animate-spin text-primary" /></div></div>
  if (!token || !status) return <Login error={loginError} theme={theme} onThemeToggle={toggleTheme} onAuthenticated={(nextToken, nextStatus) => {
    sessionStorage.setItem(sessionTokenKey, nextToken); setLoginError(""); setToken(nextToken); setStatus(nextStatus)
  }} />
  return <Dashboard initialStatus={status} theme={theme} token={token} onThemeToggle={toggleTheme} onLogout={() => {
    sessionStorage.removeItem(sessionTokenKey); setStatus(null); setToken("")
  }} />
}

function Login({ error, theme, onAuthenticated, onThemeToggle }: { error: string; theme: Theme; onAuthenticated: (token: string, status: StatusResponse) => void; onThemeToggle: () => void }) {
  const [value, setValue] = useState(""); const [visible, setVisible] = useState(false)
  const [submitting, setSubmitting] = useState(false); const [message, setMessage] = useState(error)
  useEffect(() => setMessage(error), [error])
  const submit = async (event: FormEvent) => {
    event.preventDefault(); const nextToken = value.trim(); if (!nextToken) return setMessage("请输入管理访问令牌。")
    setSubmitting(true); setMessage("")
    try { onAuthenticated(nextToken, await apiRequest<StatusResponse>("/v1/status", nextToken)) }
    catch (requestError) { setMessage(humanizeError(requestError)) } finally { setSubmitting(false) }
  }
  return <main className="auth-stage">
    <div className="ambient ambient-one" /><div className="ambient ambient-two" />
    <button className="theme-fab" type="button" onClick={onThemeToggle} aria-label="切换主题">{theme === "light" ? <Moon /> : <Sun />}</button>
    <section className="login-card" aria-labelledby="login-title"><BrandMark large />
      <div className="login-heading"><p className="eyebrow">Standalone Server</p><h1 id="login-title">登录 CMCC Notify</h1><p>使用服务端配置的管理令牌进入控制台。</p></div>
      <form onSubmit={submit} className="login-form"><label htmlFor="access-token">管理访问令牌</label>
        <InputGroup className={cn("soft-input", message && "border-destructive/70")}><InputGroupAddon><LockKeyhole /></InputGroupAddon>
          <InputGroupInput id="access-token" autoComplete="current-password" autoFocus aria-invalid={Boolean(message)} placeholder="输入 CMCC_NOTIFY_AUTH_TOKEN" type={visible ? "text" : "password"} value={value} onChange={(event) => setValue(event.target.value)} />
          <InputGroupAddon align="inline-end"><InputGroupButton onClick={() => setVisible((current) => !current)} aria-label={visible ? "隐藏令牌" : "显示令牌"}>{visible ? <EyeOff /> : <Eye />}</InputGroupButton></InputGroupAddon>
        </InputGroup><div className="login-feedback" aria-live="polite">{message ? <span className="error-text">{message}</span> : <span>令牌仅保存在当前浏览器会话中。</span>}</div>
        <Button size="lg" type="submit" className="w-full" disabled={submitting}>{submitting ? <LoaderCircle className="animate-spin" /> : <ShieldCheck />}{submitting ? "正在验证" : "进入控制台"}</Button>
      </form>
    </section><p className="auth-footer">CMCC Notify · 安全连接</p>
  </main>
}

function Dashboard({ initialStatus, onLogout, onThemeToggle, theme, token }: { initialStatus: StatusResponse; onLogout: () => void; onThemeToggle: () => void; theme: Theme; token: string }) {
  const [page, setPage] = useState<Page>("overview"); const [status, setStatus] = useState(initialStatus)
  const [settings, setSettings] = useState<SettingsResponse | null>(null); const [refreshing, setRefreshing] = useState(false)
  const [statusError, setStatusError] = useState(""); const [lastUpdated, setLastUpdated] = useState(new Date()); const [notice, setNotice] = useState("")
  const refresh = useCallback(async (quiet = false) => {
    if (!quiet) setRefreshing(true)
    try { const next = await apiRequest<StatusResponse>("/v1/status", token); setStatus(next); setStatusError(""); setLastUpdated(new Date()) }
    catch (error) { if (error instanceof APIError && error.status === 401) return onLogout(); setStatusError(humanizeError(error)) }
    finally { if (!quiet) setRefreshing(false) }
  }, [onLogout, token])
  const refreshSettings = useCallback(async () => {
    try { const next = await apiRequest<SettingsResponse>("/v1/settings", token); setSettings(next); return next }
    catch (error) { if (error instanceof APIError && error.status === 401) onLogout(); else setStatusError(humanizeError(error)); return null }
  }, [onLogout, token])
  const showNotice = useCallback((message: string) => { setNotice(message); window.setTimeout(() => setNotice(""), 2600) }, [])
  useEffect(() => { void refreshSettings() }, [refreshSettings])
  useEffect(() => { const timer = window.setInterval(() => void refresh(true), 15000); return () => window.clearInterval(timer) }, [refresh])
  const currentLabel = navigation.find((item) => item.id === page)?.label ?? "概览"
  return <div className="dashboard-shell">
    <aside className="sidebar"><div className="sidebar-brand"><BrandMark /><div><strong>CMCC Notify</strong><span>Standalone Server</span></div></div>
      <nav className="sidebar-nav" aria-label="主导航">{navigation.map((item) => <NavButton key={item.id} item={item} active={page === item.id} onClick={() => setPage(item.id)} />)}</nav>
      <div className="sidebar-footer"><button type="button" onClick={onThemeToggle}>{theme === "light" ? <Moon /> : <Sun />}<span>主题</span><small>{theme === "light" ? "浅色" : "深色"}</small></button><button type="button" onClick={onLogout}><LogOut /><span>退出</span></button><div className="version-line">{formatVersion(status.version)}</div></div>
    </aside>
    <div className="workspace"><header className="mobile-header"><div className="mobile-title"><BrandMark /><div><strong>CMCC Notify</strong><span>{currentLabel}</span></div></div><div className="mobile-actions"><button type="button" onClick={onThemeToggle} aria-label="切换主题">{theme === "light" ? <Moon /> : <Sun />}</button><button type="button" onClick={onLogout} aria-label="退出"><LogOut /></button></div></header>
      <main className="content">{statusError && <div className="inline-alert"><WifiOff />{statusError}</div>}
        {page === "overview" && <Overview status={status} refreshing={refreshing} lastUpdated={lastUpdated} onRefresh={() => void refresh()} onNavigate={setPage} />}
        {page === "send" && <SendPage status={status} settings={settings} token={token} onStatusRefresh={() => void refresh(true)} showNotice={showNotice} />}
        {page === "accounts" && <AccountsPage status={status} settings={settings} token={token} refreshing={refreshing} onRefresh={() => { void refresh(); void refreshSettings() }} onSettings={setSettings} showNotice={showNotice} />}
        {page === "docs" && <DocsPage status={status} />}
      </main>
      <nav className="bottom-nav" aria-label="移动端主导航">{navigation.map((item) => { const Icon = item.icon; return <button key={item.id} type="button" className={cn(page === item.id && "active")} onClick={() => setPage(item.id)}><Icon /><span>{item.label}</span></button> })}</nav>
    </div>{notice && <div className="toast" role="status"><CheckCircle2 />{notice}</div>}
  </div>
}

function NavButton({ active, item, onClick }: { active: boolean; item: (typeof navigation)[number]; onClick: () => void }) { const Icon = item.icon; return <button type="button" className={cn("nav-button", active && "active")} onClick={onClick} aria-current={active ? "page" : undefined}><Icon /><span>{item.label}</span></button> }

function Overview({ lastUpdated, onNavigate, onRefresh, refreshing, status }: { lastUpdated: Date; onNavigate: (page: Page) => void; onRefresh: () => void; refreshing: boolean; status: StatusResponse }) {
  const enabled = status.accounts.filter((account) => account.enabled); const connected = enabled.filter((account) => account.connected).length; const healthy = status.status === "ok"
  const stats = [
    { label: "服务状态", value: healthy ? "运行正常" : "状态异常", detail: healthy ? "HTTP 服务可用" : status.status, icon: healthy ? CheckCircle2 : WifiOff, tone: healthy ? "green" : "red" },
    { label: "Server 版本", value: formatVersion(status.version), detail: "当前运行版本", icon: ServerIcon, tone: "blue" },
    { label: "运行时间", value: formatUptime(status.uptime_seconds), detail: "本次启动以来", icon: Clock3, tone: "violet" },
    { label: "已连接通道", value: `${connected} / ${enabled.length}`, detail: enabled.length ? "已启用通道" : "尚未启用通道", icon: Wifi, tone: connected === enabled.length && enabled.length > 0 ? "green" : "amber" },
  ]
  return <PageFrame eyebrow="Dashboard" title="概览" description="查看 CMCC Notify 当前的运行与通道连接状态。" action={<Button variant="outline" onClick={onRefresh} disabled={refreshing}><RefreshCw className={cn(refreshing && "animate-spin")} />刷新状态</Button>}>
    <section className="stat-grid" aria-label="服务状态概览">{stats.map((stat) => { const Icon = stat.icon; return <article className="soft-card stat-card" key={stat.label}><div className={cn("icon-tile", `tone-${stat.tone}`)}><Icon /></div><div className="stat-copy"><span>{stat.label}</span><strong>{stat.value}</strong><small>{stat.detail}</small></div></article> })}</section>
    <section className="overview-grid"><article className="soft-card section-card account-summary"><SectionHeading icon={<Radio />} title="通道连接" description={`状态更新于 ${lastUpdated.toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit", second: "2-digit" })}`} /><div className="account-rows">{status.accounts.length ? status.accounts.map((account) => <AccountRow key={account.name} account={account} compact />) : <EmptyState icon={<UsersRound />} title="尚未配置通道" description="进入账户状态即可添加第一个 CMCC 通道。" />}</div><Button variant="outline" className="mt-5 w-full" onClick={() => onNavigate("accounts")}>管理通道与号码组</Button></article>
      <article className="soft-card section-card quick-send"><SectionHeading icon={<MessageSquareText />} title="联通测试" description="用 WebUI 验证通道、目标号码和多媒体链路。" /><div className="quick-illustration"><Send /><span /></div><p>生产通知建议在“账户状态 → 通知应用”创建独立 Token，再由业务系统调用 <code>/v1/notify</code>。</p><Button size="lg" onClick={() => onNavigate("send")}><Send />测试发送</Button></article>
    </section>
  </PageFrame>
}

function SendPage({ status, settings, token, onStatusRefresh, showNotice }: { status: StatusResponse; settings: SettingsResponse | null; token: string; onStatusRefresh: () => void; showNotice: (message: string) => void }) {
  const accountSource = settings?.accounts ?? status.accounts; const availableAccounts = accountSource.filter((item) => item.enabled); const groups = settings?.groups ?? []
  const [account, setAccount] = useState(availableAccounts[0]?.name ?? ""); const [mode, setMode] = useState<"text" | "media">("text")
  const [recipientMode, setRecipientMode] = useState<"single" | "group">("single"); const [mediaSource, setMediaSource] = useState<"upload" | "url">("upload")
  const [mediaType, setMediaType] = useState<MediaType>("AUTO"); const [to, setTo] = useState(""); const [group, setGroup] = useState(groups[0]?.name ?? "")
  const [text, setText] = useState(""); const [mediaURL, setMediaURL] = useState(""); const [file, setFile] = useState<File | null>(null)
  const [submitting, setSubmitting] = useState(false); const [error, setError] = useState(""); const [result, setResult] = useState<SendResponse | null>(null)
  const selectedAccount = availableAccounts.find((item) => item.name === account)
  useEffect(() => { if (!availableAccounts.some((item) => item.name === account)) setAccount(availableAccounts[0]?.name ?? "") }, [account, availableAccounts])
  useEffect(() => { if (!groups.some((item) => item.name === group)) setGroup(groups[0]?.name ?? "") }, [group, groups])
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setError(""); setResult(null)
    if (!account) return setError("没有可用通道，请先添加并启用一个 CMCC 通道。")
    if (mode === "text" && !text.trim()) return setError("消息内容不能为空。")
    if (recipientMode === "group" && !group) return setError("请先选择本地号码组。")
    if (mode === "media" && mediaSource === "upload" && !file) return setError("请选择要上传的文件。")
    if (mode === "media" && mediaSource === "url" && !mediaURL.trim()) return setError("多媒体消息必须填写文件 URL。")
    setSubmitting(true)
    try {
      let response: SendResponse
      if (mode === "text") {
        const payload = recipientMode === "group" ? { account, group, text: text.trim() } : { account, to: to.trim(), text: text.trim() }
        response = await apiRequest<SendResponse>("/v1/send", token, { method: "POST", body: JSON.stringify(payload) })
      } else if (mediaSource === "upload") {
        const form = new FormData(); form.set("account", account); form.set("type", mediaType); form.set("caption", text.trim()); form.set("file", file as File); form.set(recipientMode === "group" ? "group" : "to", recipientMode === "group" ? group : to.trim())
        response = await apiRequest<SendResponse>("/v1/send/media", token, { method: "POST", body: form })
      } else response = await apiRequest<SendResponse>("/v1/send", token, { method: "POST", body: JSON.stringify({ account, ...(recipientMode === "group" ? { group } : { to: to.trim() }), media: { type: mediaType === "AUTO" ? "FILE" : mediaType, url: mediaURL.trim(), caption: text.trim() } }) })
      setResult(response); showNotice(response.failed_count ? `本地扇出完成：成功 ${response.accepted_count ?? 0}，失败 ${response.failed_count}` : mode === "media" && mediaSource === "upload" ? "文件已上传并提交到 CMCC" : "消息已提交到 CMCC"); onStatusRefresh()
    } catch (requestError) { setError(humanizeError(requestError)) } finally { setSubmitting(false) }
  }
  return <PageFrame eyebrow="Compose" title="发送消息" description="向单个目标发送，或通过本地号码组逐个扇出文字与多媒体消息。"><div className="send-layout">
    <form className="soft-card section-card send-form" onSubmit={submit}>
      <SegmentedControl value={mode} options={[{ value: "text", label: "文字消息", icon: <FileText /> }, { value: "media", label: "多媒体", icon: <ImageIcon /> }]} onChange={(value) => setMode(value as "text" | "media")} />
      <Field label="发送通道" htmlFor="send-account" hint={selectedAccount?.connected ? "通道当前已连接" : "发送时服务会尝试建立连接"}><Select id="send-account" value={account} onValueChange={setAccount} disabled={!availableAccounts.length} emptyLabel="无可用通道" icon={<UsersRound />} options={availableAccounts.map((item) => ({ value: item.name, label: item.name, description: item.default_to ? `默认目标 ${item.default_to}` : "未配置默认目标" }))} endAdornment={<span className={cn("select-status", selectedAccount?.connected && "online")}>{selectedAccount?.connected ? "已连接" : "未连接"}</span>} /></Field>
      <SegmentedControl value={recipientMode} options={[{ value: "single", label: "单个目标", icon: <Smartphone /> }, { value: "group", label: "本地号码组", icon: <UsersRound /> }]} onChange={(value) => setRecipientMode(value as "single" | "group")} />
      {recipientMode === "single" ? <Field label="目标号码（to）" htmlFor="send-to" hint={selectedAccount?.default_to ? `留空时使用通道默认目标 ${selectedAccount.default_to}` : "未配置默认目标时此项必填"}><InputGroup className="soft-input"><InputGroupAddon><Smartphone /></InputGroupAddon><InputGroupInput id="send-to" inputMode="tel" placeholder="例如：13800138000" value={to} onChange={(event) => setTo(event.target.value)} /></InputGroup></Field>
        : <Field label="本地号码组" htmlFor="send-group" hint={group ? `${groups.find((item) => item.name === group)?.recipients.length ?? 0} 个 to 目标，将逐个发送` : "请先在账户状态中创建号码组"}><Select id="send-group" value={group} onValueChange={setGroup} disabled={!groups.length} emptyLabel="暂无号码组" icon={<UsersRound />} options={groups.map((item) => ({ value: item.name, label: item.name, description: `${item.recipients.length} 个 to 目标 · 本地扇出` }))} /></Field>}
      {recipientMode === "group" && <div className="info-note"><Network />号码组不是 CMCC 原生群发。服务端会使用当前通道，对组内每个 <code>to</code> 目标分别发送一条消息。</div>}
      {mode === "media" && <><SegmentedControl value={mediaSource} options={[{ value: "upload", label: "本地上传", icon: <FileUp /> }, { value: "url", label: "远程 URL", icon: <Link2 /> }]} onChange={(value) => { const source = value as "upload" | "url"; setMediaSource(source); if (source === "url" && mediaType === "AUTO") setMediaType("IMAGE") }} />
          <div className="two-fields"><Field label="媒体类型" htmlFor="media-type"><Select id="media-type" value={mediaType} onValueChange={(value) => setMediaType(value as MediaType)} icon={<ImageIcon />} options={[{ value: "AUTO", label: "自动识别", description: "根据文件类型判断" }, { value: "IMAGE", label: "图片" }, { value: "AUDIO", label: "音频" }, { value: "VIDEO", label: "视频" }, { value: "FILE", label: "文件" }]} /></Field>{mediaSource === "url" && <Field label="文件 URL" htmlFor="media-url" hint="建议使用可公开访问的 HTTPS 地址"><InputGroup className="soft-input"><InputGroupAddon><Link2 /></InputGroupAddon><InputGroupInput id="media-url" inputMode="url" placeholder="https://example.com/file.jpg" value={mediaURL} onChange={(event) => setMediaURL(event.target.value)} /></InputGroup></Field>}</div>
          {mediaSource === "upload" && <Field label="选择文件" htmlFor="media-file" hint="最大 200 MiB；将先上传到 CMCC 再发送"><label className={cn("file-picker", file && "selected")} htmlFor="media-file"><input id="media-file" type="file" onChange={(event) => setFile(event.target.files?.[0] ?? null)} /><div><FileUp /><span><strong>{file?.name ?? "点击选择本地文件"}</strong><small>{file ? `${file.type || "未知类型"} · ${formatBytes(file.size)}` : "图片、音频、视频或任意文件"}</small></span></div>{file && <CheckCircle2 />}</label></Field>}</>}
      <Field label={mode === "text" ? "消息内容" : "附言（可选）"} htmlFor="message-text"><InputGroup className="soft-input message-input"><InputGroupTextarea id="message-text" maxLength={2000} rows={7} placeholder={mode === "text" ? "输入要发送的消息…" : "为文件添加一段说明…"} value={text} onChange={(event) => setText(event.target.value)} /><InputGroupAddon align="block-end" className="justify-end border-t border-border/60"><InputGroupText>{text.length} / 2000</InputGroupText></InputGroupAddon></InputGroup></Field>
      {mode === "media" && <div className="info-note"><ShieldCheck />本地文件会先上传到 CMCC，再通过带 <code>to</code> 的富媒体帧发送。图片和 ZIP 文件已于 2026-09-16 完成真实终端验证；号码组模式只上传一次，再逐个目标提交。</div>}
      {error && <div className="form-error" role="alert"><AlertTriangle />{error}</div>}<Button type="submit" size="lg" className="w-full" disabled={submitting || !availableAccounts.length}>{submitting ? <LoaderCircle className="animate-spin" /> : <Send />}{submitting ? mode === "media" && mediaSource === "upload" ? "正在上传并提交" : "正在提交" : "提交到 CMCC 网关"}</Button>
    </form>
    <aside className="soft-card section-card result-panel"><SectionHeading icon={<Activity />} title="提交结果" description="显示网关写入结果，不代表送达或已读。" />
      {result ? <div className={cn("success-result", result.failed_count && "partial")} aria-live="polite"><div className="success-orb">{result.failed_count ? <AlertTriangle /> : <Check />}</div><h3>{result.group ? result.failed_count ? "本地扇出部分失败" : "本地扇出已完成" : "CMCC 网关已接受"}</h3><p>{result.group ? `${result.group}：网关写入成功 ${result.accepted_count ?? 0}，失败 ${result.failed_count ?? 0}` : "请求已通过 CMCC 消息通道写入。"}</p><dl>{result.mode && <div><dt>发送方式</dt><dd>本地逐号码扇出</dd></div>}{result.message_id && <div><dt>Message ID</dt><dd>{result.message_id}</dd></div>}{result.media_type && <div><dt>媒体类型</dt><dd>{result.media_type}</dd></div>}{result.file_name && <div><dt>文件</dt><dd>{result.file_name}</dd></div>}{result.size !== undefined && <div><dt>大小</dt><dd>{formatBytes(result.size)}</dd></div>}{result.total !== undefined && <div><dt>目标数量</dt><dd>{result.total}</dd></div>}{result.mode && <div><dt>CMCC 原生广播</dt><dd>{result.native_broadcast ? "是" : "否"}</dd></div>}<div><dt>送达回执</dt><dd>{result.acknowledged ? "是" : "未提供"}</dd></div></dl>{result.results?.length ? <div className="fanout-results">{result.results.map((item) => <div className={cn(item.error ? "failed" : "accepted")} key={`${item.to}-${item.message_id ?? item.error}`}><span>{item.to}</span><strong>{item.error ? "写入失败" : "已写入网关"}</strong></div>)}</div> : null}</div> : <EmptyState icon={<Send />} title="等待发送" description="填写左侧表单并提交后，上传和网关结果会显示在这里。" />}
    </aside>
  </div></PageFrame>
}

function AccountsPage({ status, settings, token, refreshing, onRefresh, onSettings, showNotice }: { status: StatusResponse; settings: SettingsResponse | null; token: string; refreshing: boolean; onRefresh: () => void; onSettings: (settings: SettingsResponse) => void; showNotice: (message: string) => void }) {
  const [tab, setTab] = useState<"accounts" | "groups" | "applications">("accounts"); const [accountEditor, setAccountEditor] = useState<AccountSetting | "new" | null>(null)
  const [groupEditor, setGroupEditor] = useState<RecipientGroup | "new" | null>(null); const [applicationEditor, setApplicationEditor] = useState<ApplicationSetting | "new" | null>(null)
  const [applicationToken, setApplicationToken] = useState<{ name: string; token: string } | null>(null); const [busy, setBusy] = useState(""); const [error, setError] = useState("")
  const accounts: AccountSetting[] = settings?.accounts ?? status.accounts.map((account) => ({ ...account, has_api_key: Boolean(account.api_key) })); const groups = settings?.groups ?? []; const applications = settings?.applications ?? []
  const connected = accounts.filter((account) => account.enabled && account.connected).length; const writable = settings?.writable ?? false
  const cleanSettings = (next: SettingsResponse): SettingsResponse => ({ ...next, application_token: undefined })
  const removeAccount = async (account: AccountSetting) => {
    if (!window.confirm(`确定删除通道“${account.name}”吗？`)) return; setBusy(`account:${account.name}`); setError("")
    try { await apiRequest<void>(`/v1/accounts/${encodeURIComponent(account.name)}`, token, { method: "DELETE" }); const next = await apiRequest<SettingsResponse>("/v1/settings", token); onSettings(next); showNotice(`通道“${account.name}”已删除`); onRefresh() }
    catch (requestError) { setError(humanizeError(requestError)) } finally { setBusy("") }
  }
  const removeGroup = async (group: RecipientGroup) => {
    if (!window.confirm(`确定删除号码组“${group.name}”吗？`)) return; setBusy(`group:${group.name}`); setError("")
    try { await apiRequest<void>(`/v1/groups/${encodeURIComponent(group.name)}`, token, { method: "DELETE" }); const next = await apiRequest<SettingsResponse>("/v1/settings", token); onSettings(next); showNotice(`号码组“${group.name}”已删除`) }
    catch (requestError) { setError(humanizeError(requestError)) } finally { setBusy("") }
  }
  const removeApplication = async (application: ApplicationSetting) => {
    if (!window.confirm(`确定删除应用“${application.name}”并立即吊销它的 Token 吗？`)) return; setBusy(`application:${application.name}`); setError("")
    try { await apiRequest<void>(`/v1/applications/${encodeURIComponent(application.name)}`, token, { method: "DELETE" }); const next = await apiRequest<SettingsResponse>("/v1/settings", token); onSettings(next); showNotice(`应用“${application.name}”已删除`) }
    catch (requestError) { setError(humanizeError(requestError)) } finally { setBusy("") }
  }
  const rotateApplication = async (application: ApplicationSetting) => {
    if (!window.confirm(`轮换“${application.name}”的 Token 后，旧 Token 会立即失效。继续吗？`)) return; setBusy(`rotate:${application.name}`); setError("")
    try { const next = await apiRequest<SettingsResponse>(`/v1/applications/${encodeURIComponent(application.name)}/rotate-token`, token, { method: "POST" }); if (!next.application_token) throw new Error("服务未返回新应用 Token"); onSettings(cleanSettings(next)); setApplicationToken({ name: application.name, token: next.application_token }); showNotice(`应用“${application.name}”的 Token 已轮换`) }
    catch (requestError) { setError(humanizeError(requestError)) } finally { setBusy("") }
  }
  const create = () => { if (tab === "accounts") setAccountEditor("new"); else if (tab === "groups") setGroupEditor("new"); else setApplicationEditor("new") }
  return <PageFrame eyebrow="Connections" title="账户状态" description="管理 CMCC 通道、本地号码组与供业务系统调用的通知应用。" action={<Button variant="outline" onClick={onRefresh} disabled={refreshing}><RefreshCw className={cn(refreshing && "animate-spin")} />刷新状态</Button>}>
    <div className="account-count-line"><span><Wifi />已连接</span><strong>{connected}</strong><span className="divider" /><span><UsersRound />通道</span><strong>{accounts.length}</strong><span className="divider" /><span><UsersRound />号码组</span><strong>{groups.length}</strong><span className="divider" /><span><BellRing />应用</span><strong>{applications.length}</strong></div>
    <div className="management-toolbar"><SegmentedControl value={tab} options={[{ value: "accounts", label: "CMCC 通道", icon: <Radio /> }, { value: "groups", label: "本地号码组", icon: <UsersRound /> }, { value: "applications", label: "通知应用", icon: <BellRing /> }]} onChange={(value) => setTab(value as "accounts" | "groups" | "applications")} /><Button onClick={create} disabled={!writable}><Plus />{tab === "accounts" ? "添加通道" : tab === "groups" ? "新建号码组" : "新建应用"}</Button></div>
    {!writable && <div className="info-note"><LockKeyhole />当前服务未配置 <code>state_file</code>，管理功能为只读。配置后即可在 WebUI 中新增、编辑和删除。</div>}{error && <div className="form-error"><AlertTriangle />{error}</div>}
    {tab === "accounts" ? <section className="account-grid">{accounts.length ? accounts.map((account) => <article className="soft-card account-card" key={account.name}><AccountRow account={account} /><dl className="account-details"><div><dt>Channel API Key</dt><dd>{account.api_key || "未配置"}</dd></div><div><dt>默认 to 目标</dt><dd>{account.default_to || "未配置"}</dd></div><div><dt>配置状态</dt><dd>{account.enabled ? "已启用" : "已停用"}</dd></div><div><dt>上传地址</dt><dd>{account.upload_url || "官方默认"}</dd></div></dl><div className="card-actions"><Button variant="outline" size="sm" onClick={() => setAccountEditor(account)} disabled={!writable}><Pencil />编辑</Button><Button variant="destructive" size="sm" onClick={() => void removeAccount(account)} disabled={!writable || busy === `account:${account.name}`}>{busy === `account:${account.name}` ? <LoaderCircle className="animate-spin" /> : <Trash2 />}删除</Button></div></article>) : <article className="soft-card section-card"><EmptyState icon={<UsersRound />} title="尚未配置通道" description="点击“添加通道”即可直接在 WebUI 中创建。" /></article>}</section>
      : tab === "groups" ? <section className="account-grid">{groups.length ? groups.map((group) => <article className="soft-card account-card group-card" key={group.name}><div className="group-heading"><div className="account-avatar online"><UsersRound /></div><div><strong>{group.name}</strong><span>{group.recipients.length} 个 to 目标 · 本地扇出</span></div></div><div className="recipient-chips">{group.recipients.map((recipient) => <span key={recipient}>{recipient}</span>)}</div><div className="card-actions"><Button variant="outline" size="sm" onClick={() => setGroupEditor(group)} disabled={!writable}><Pencil />编辑</Button><Button variant="destructive" size="sm" onClick={() => void removeGroup(group)} disabled={!writable || busy === `group:${group.name}`}>{busy === `group:${group.name}` ? <LoaderCircle className="animate-spin" /> : <Trash2 />}删除</Button></div></article>) : <article className="soft-card section-card"><EmptyState icon={<UsersRound />} title="还没有本地号码组" description="将多个 to 目标保存为号码组后，可在发送页面和通知应用中逐个扇出。" /></article>}</section>
        : <section className="account-grid">{applications.length ? applications.map((application) => <article className="soft-card account-card application-card" key={application.name}><div className="group-heading"><div className={cn("account-avatar", application.enabled ? "online" : "offline")}><BellRing /></div><div><strong>{application.name}</strong><span>{application.enabled ? "已启用" : "已停用"} · {application.token_hint}</span></div></div><dl className="account-details"><div><dt>CMCC 通道</dt><dd>{application.account}</dd></div><div><dt>固定目标</dt><dd>{application.group ? `本地号码组：${application.group}` : application.to}</dd></div><div><dt>每分钟限额</dt><dd>{application.rate_limit_per_minute} 次</dd></div><div><dt>目标覆盖</dt><dd>{application.allow_recipient_override ? "允许" : "禁止"}</dd></div></dl><div className="card-actions application-actions"><Button variant="outline" size="sm" onClick={() => setApplicationEditor(application)} disabled={!writable}><Pencil />编辑</Button><Button variant="outline" size="sm" onClick={() => void rotateApplication(application)} disabled={!writable || busy === `rotate:${application.name}`}>{busy === `rotate:${application.name}` ? <LoaderCircle className="animate-spin" /> : <RotateCcw />}轮换 Token</Button><Button variant="destructive" size="sm" onClick={() => void removeApplication(application)} disabled={!writable || busy === `application:${application.name}`}>{busy === `application:${application.name}` ? <LoaderCircle className="animate-spin" /> : <Trash2 />}删除</Button></div></article>) : <article className="soft-card section-card"><EmptyState icon={<BellRing />} title="还没有通知应用" description="创建应用后，业务系统可使用独立 Token 调用 /v1/notify，无需接触管理令牌和 CMCC API Key。" /></article>}</section>}
    <div className="info-note"><KeyRound />应用 Token 只在创建或轮换时显示一次，服务端仅保存哈希。默认固定通道和目标，并禁止调用方临时修改；需要动态路由时可为特定应用单独开启。</div>
    {accountEditor && <AccountEditor token={token} existing={accountEditor === "new" ? null : accountEditor} onClose={() => setAccountEditor(null)} onSaved={(next, name) => { onSettings(next); setAccountEditor(null); showNotice(`通道“${name}”已保存`); onRefresh() }} />}
    {groupEditor && <GroupEditor token={token} existing={groupEditor === "new" ? null : groupEditor} onClose={() => setGroupEditor(null)} onSaved={(next, name) => { onSettings(next); setGroupEditor(null); showNotice(`号码组“${name}”已保存`) }} />}
    {applicationEditor && <ApplicationEditor token={token} accounts={accounts} groups={groups} existing={applicationEditor === "new" ? null : applicationEditor} onClose={() => setApplicationEditor(null)} onSaved={(next, name) => { onSettings(cleanSettings(next)); setApplicationEditor(null); if (next.application_token) setApplicationToken({ name, token: next.application_token }); showNotice(`应用“${name}”已保存`) }} />}
    {applicationToken && <ApplicationTokenDialog application={applicationToken.name} token={applicationToken.token} onClose={() => setApplicationToken(null)} />}
  </PageFrame>
}

function AccountEditor({ token, existing, onClose, onSaved }: { token: string; existing: AccountSetting | null; onClose: () => void; onSaved: (settings: SettingsResponse, name: string) => void }) {
  const [name, setName] = useState(existing?.name ?? ""); const [apiKey, setAPIKey] = useState(""); const [enabled, setEnabled] = useState(existing?.enabled ?? true)
  const [defaultTo, setDefaultTo] = useState(existing?.default_to ?? ""); const [serverURL, setServerURL] = useState(existing?.server_url ?? ""); const [uploadURL, setUploadURL] = useState(existing?.upload_url ?? "")
  const [saving, setSaving] = useState(false); const [error, setError] = useState("")
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setSaving(true); setError("")
    try { const path = existing ? `/v1/accounts/${encodeURIComponent(existing.name)}` : "/v1/accounts"; const next = await apiRequest<SettingsResponse>(path, token, { method: existing ? "PUT" : "POST", body: JSON.stringify({ name: name.trim(), api_key: apiKey.trim(), enabled, default_to: defaultTo.trim(), server_url: serverURL.trim(), upload_url: uploadURL.trim() }) }); onSaved(next, name.trim()) }
    catch (requestError) { setError(humanizeError(requestError)) } finally { setSaving(false) }
  }
  return <Dialog title={existing ? `编辑通道 · ${existing.name}` : "添加 CMCC 通道"} description="一条通道对应一份 Channel API Key；保存后连接会自动重建。" onClose={onClose}><form className="editor-form" onSubmit={submit}>
    <Field label="通道名称" htmlFor="account-name" hint={existing ? "重命名会同步更新关联通知应用" : undefined}><InputGroup className="soft-input"><InputGroupAddon><Radio /></InputGroupAddon><InputGroupInput id="account-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="例如：primary" required /></InputGroup></Field>
    <Field label="Channel API Key" htmlFor="account-key" hint={existing ? "留空保留当前密钥" : "启用通道时必填"}><InputGroup className="soft-input"><InputGroupAddon><KeyRound /></InputGroupAddon><InputGroupInput id="account-key" type="password" autoComplete="new-password" value={apiKey} onChange={(event) => setAPIKey(event.target.value)} placeholder={existing?.api_key || "ak_..."} /></InputGroup></Field>
    <Field label="默认目标号码（to）" htmlFor="account-default" hint="这是发送路由数据，不是备注"><InputGroup className="soft-input"><InputGroupAddon><Smartphone /></InputGroupAddon><InputGroupInput id="account-default" inputMode="tel" value={defaultTo} onChange={(event) => setDefaultTo(event.target.value)} placeholder="可选" /></InputGroup></Field>
    <details className="advanced-fields"><summary>高级协议地址</summary><div><Field label="WebSocket 地址" htmlFor="account-server"><InputGroup className="soft-input"><InputGroupAddon><Link2 /></InputGroupAddon><InputGroupInput id="account-server" value={serverURL} onChange={(event) => setServerURL(event.target.value)} placeholder="留空使用官方默认" /></InputGroup></Field><Field label="上传 API 地址" htmlFor="account-upload"><InputGroup className="soft-input"><InputGroupAddon><Link2 /></InputGroupAddon><InputGroupInput id="account-upload" value={uploadURL} onChange={(event) => setUploadURL(event.target.value)} placeholder="留空使用官方默认" /></InputGroup></Field></div></details>
    <label className="toggle-row"><span><strong>启用通道</strong><small>保存后立即连接 CMCC 网关</small></span><input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} /></label>
    {error && <div className="form-error"><AlertTriangle />{error}</div>}<div className="dialog-actions"><Button variant="outline" onClick={onClose}>取消</Button><Button type="submit" disabled={saving}>{saving ? <LoaderCircle className="animate-spin" /> : <Save />}{saving ? "正在保存" : "保存通道"}</Button></div>
  </form></Dialog>
}

function GroupEditor({ token, existing, onClose, onSaved }: { token: string; existing: RecipientGroup | null; onClose: () => void; onSaved: (settings: SettingsResponse, name: string) => void }) {
  const [name, setName] = useState(existing?.name ?? ""); const [recipients, setRecipients] = useState(existing?.recipients.join("\n") ?? "")
  const [saving, setSaving] = useState(false); const [error, setError] = useState("")
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setSaving(true); setError(""); const values = recipients.split(/[\n,，;；]+/).map((item) => item.trim()).filter(Boolean)
    try { const path = existing ? `/v1/groups/${encodeURIComponent(existing.name)}` : "/v1/groups"; const next = await apiRequest<SettingsResponse>(path, token, { method: existing ? "PUT" : "POST", body: JSON.stringify({ name: name.trim(), recipients: values }) }); onSaved(next, name.trim()) }
    catch (requestError) { setError(humanizeError(requestError)) } finally { setSaving(false) }
  }
  return <Dialog title={existing ? `编辑号码组 · ${existing.name}` : "新建本地号码组"} description="每行填写一个 to 目标。发送时会逐个写入，不是 CMCC 原生群发。" onClose={onClose}><form className="editor-form" onSubmit={submit}>
    <Field label="号码组名称" htmlFor="group-name"><InputGroup className="soft-input"><InputGroupAddon><UsersRound /></InputGroupAddon><InputGroupInput id="group-name" value={name} disabled={Boolean(existing)} onChange={(event) => setName(event.target.value)} placeholder="例如：服务器管理员" required /></InputGroup></Field>
    <Field label="目标号码（to）" htmlFor="group-recipients" hint="重复目标会自动去重"><InputGroup className="soft-input message-input"><InputGroupTextarea id="group-recipients" rows={8} value={recipients} onChange={(event) => setRecipients(event.target.value)} placeholder={"13800138000\n13900139000"} required /></InputGroup></Field>
    {error && <div className="form-error"><AlertTriangle />{error}</div>}<div className="dialog-actions"><Button variant="outline" onClick={onClose}>取消</Button><Button type="submit" disabled={saving}>{saving ? <LoaderCircle className="animate-spin" /> : <Save />}{saving ? "正在保存" : "保存号码组"}</Button></div>
  </form></Dialog>
}

function ApplicationEditor({ token, accounts, groups, existing, onClose, onSaved }: { token: string; accounts: AccountSetting[]; groups: RecipientGroup[]; existing: ApplicationSetting | null; onClose: () => void; onSaved: (settings: SettingsResponse, name: string) => void }) {
  const [name, setName] = useState(existing?.name ?? ""); const [account, setAccount] = useState(existing?.account ?? accounts[0]?.name ?? "")
  const [recipientMode, setRecipientMode] = useState<"single" | "group">(existing?.group ? "group" : "single"); const [to, setTo] = useState(existing?.to ?? ""); const [group, setGroup] = useState(existing?.group ?? groups[0]?.name ?? "")
  const [enabled, setEnabled] = useState(existing?.enabled ?? true); const [allowOverride, setAllowOverride] = useState(existing?.allow_recipient_override ?? false); const [rateLimit, setRateLimit] = useState(existing?.rate_limit_per_minute ?? 60)
  const [saving, setSaving] = useState(false); const [error, setError] = useState("")
  const submit = async (event: FormEvent) => {
    event.preventDefault(); setError("")
    if (!account) return setError("请先选择一个 CMCC 通道。")
    if (recipientMode === "single" && !to.trim()) return setError("应用必须绑定一个目标号码。")
    if (recipientMode === "group" && !group) return setError("应用必须绑定一个本地号码组。")
    setSaving(true)
    try {
      const path = existing ? `/v1/applications/${encodeURIComponent(existing.name)}` : "/v1/applications"
      const next = await apiRequest<SettingsResponse>(path, token, { method: existing ? "PUT" : "POST", body: JSON.stringify({ name: name.trim(), enabled, account, to: recipientMode === "single" ? to.trim() : "", group: recipientMode === "group" ? group : "", allow_recipient_override: allowOverride, rate_limit_per_minute: rateLimit }) })
      onSaved(next, name.trim())
    } catch (requestError) { setError(humanizeError(requestError)) } finally { setSaving(false) }
  }
  return <Dialog title={existing ? `编辑应用 · ${existing.name}` : "新建通知应用"} description="应用使用独立 Token 调用通知接口，不会接触管理令牌或 CMCC API Key。" onClose={onClose}><form className="editor-form" onSubmit={submit}>
    <Field label="应用名称" htmlFor="application-name"><InputGroup className="soft-input"><InputGroupAddon><BellRing /></InputGroupAddon><InputGroupInput id="application-name" value={name} disabled={Boolean(existing)} onChange={(event) => setName(event.target.value)} placeholder="例如：monitoring" required /></InputGroup></Field>
    <Field label="发送通道" htmlFor="application-account"><Select id="application-account" value={account} onValueChange={setAccount} disabled={!accounts.length} emptyLabel="暂无通道" icon={<Radio />} options={accounts.map((item) => ({ value: item.name, label: item.name, description: item.enabled ? item.connected ? "已连接" : "已启用 · 未连接" : "已停用" }))} /></Field>
    <SegmentedControl value={recipientMode} options={[{ value: "single", label: "固定 to 目标", icon: <Smartphone /> }, { value: "group", label: "固定号码组", icon: <UsersRound /> }]} onChange={(value) => setRecipientMode(value as "single" | "group")} />
    {recipientMode === "single" ? <Field label="目标号码（to）" htmlFor="application-to" hint="业务系统默认不能修改"><InputGroup className="soft-input"><InputGroupAddon><Smartphone /></InputGroupAddon><InputGroupInput id="application-to" inputMode="tel" value={to} onChange={(event) => setTo(event.target.value)} placeholder="例如：13800138000" required /></InputGroup></Field>
      : <Field label="本地号码组" htmlFor="application-group" hint="号码组成员变更会自动生效并逐个扇出"><Select id="application-group" value={group} onValueChange={setGroup} disabled={!groups.length} emptyLabel="暂无号码组" icon={<UsersRound />} options={groups.map((item) => ({ value: item.name, label: item.name, description: `${item.recipients.length} 个 to 目标 · 本地扇出` }))} /></Field>}
    <Field label="每分钟请求上限" htmlFor="application-rate" hint="1–1000 次"><InputGroup className="soft-input"><InputGroupAddon><Activity /></InputGroupAddon><InputGroupInput id="application-rate" type="number" min={1} max={1000} value={rateLimit} onChange={(event) => setRateLimit(Number(event.target.value))} required /></InputGroup></Field>
    <label className="toggle-row"><span><strong>允许调用方覆盖目标</strong><small>默认关闭；仅为确实需要动态 to 或号码组的可信应用开启</small></span><input type="checkbox" checked={allowOverride} onChange={(event) => setAllowOverride(event.target.checked)} /></label>
    <label className="toggle-row"><span><strong>启用应用</strong><small>停用后 Token 立即无法发送通知</small></span><input type="checkbox" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} /></label>
    {error && <div className="form-error"><AlertTriangle />{error}</div>}<div className="dialog-actions"><Button variant="outline" onClick={onClose}>取消</Button><Button type="submit" disabled={saving || !accounts.length}>{saving ? <LoaderCircle className="animate-spin" /> : <Save />}{saving ? "正在保存" : "保存应用"}</Button></div>
  </form></Dialog>
}

function ApplicationTokenDialog({ application, token, onClose }: { application: string; token: string; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => { await navigator.clipboard.writeText(token); setCopied(true); window.setTimeout(() => setCopied(false), 1600) }
  return <Dialog title={`保存应用 Token · ${application}`} description="这是唯一一次显示完整 Token；关闭后只能轮换，无法再次查看。" onClose={onClose}>
    <div className="token-reveal"><div className="info-note"><ShieldCheck />服务端只保存 Token 的 SHA-256 哈希。请把完整 Token 写入调用系统的 Secret，不要放进 URL、源码或日志。</div><div className="code-block"><pre>{token}</pre><button type="button" onClick={() => void copy()} aria-label="复制应用 Token">{copied ? <Check /> : <Copy />}</button></div><div className="dialog-actions"><Button onClick={onClose}>我已安全保存</Button></div></div>
  </Dialog>
}

function Dialog({ title, description, children, onClose }: { title: string; description: string; children: ReactNode; onClose: () => void }) { return <div className="dialog-backdrop" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}><section className="dialog-card" role="dialog" aria-modal="true" aria-labelledby="dialog-title"><header><div><h2 id="dialog-title">{title}</h2><p>{description}</p></div><button type="button" onClick={onClose} aria-label="关闭"><X /></button></header>{children}</section></div> }

function DocsPage({ status }: { status: StatusResponse }) {
  const [tab, setTab] = useState<"guide" | "concepts" | "api">("guide"); const [copied, setCopied] = useState("")
  const baseURL = window.location.origin; const exampleAccount = status.accounts.find((account) => account.enabled)?.name ?? "primary"
  const examples = useMemo(() => [
    { title: "服务状态", method: "GET", path: "/healthz", auth: false, code: `curl ${baseURL}/healthz` },
    { title: "后台配置", method: "GET", path: "/v1/settings", auth: true, code: `curl ${baseURL}/v1/settings \\\n  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>"` },
    { title: "添加 CMCC 通道", method: "POST", path: "/v1/accounts", auth: true, code: `curl -X POST ${baseURL}/v1/accounts \\\n  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>" \\\n  -H "Content-Type: application/json" \\\n  -d '{"name":"backup","api_key":"ak_...","enabled":true,"default_to":"13800138000"}'` },
    { title: "创建本地号码组", method: "POST", path: "/v1/groups", auth: true, code: `curl -X POST ${baseURL}/v1/groups \\\n  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>" \\\n  -H "Content-Type: application/json" \\\n  -d '{"name":"family","recipients":["13800138000","13900139000"]}'` },
    { title: "创建通知应用", method: "POST", path: "/v1/applications", auth: true, code: `curl -X POST ${baseURL}/v1/applications \\\n  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>" \\\n  -H "Content-Type: application/json" \\\n  -d '{"name":"monitoring","enabled":true,"account":"${exampleAccount}","to":"13800138000","rate_limit_per_minute":60}'` },
    { title: "业务系统发送通知", method: "POST", path: "/v1/notify", auth: true, code: `curl -X POST ${baseURL}/v1/notify \\\n  -H "Authorization: Bearer <APPLICATION_TOKEN>" \\\n  -H "Content-Type: application/json" \\\n  -d '{"title":"服务告警","message":"磁盘使用率超过 90%"}'` },
    { title: "发送文字消息", method: "POST", path: "/v1/send", auth: true, code: `curl -X POST ${baseURL}/v1/send \\\n  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>" \\\n  -H "Content-Type: application/json" \\\n  -d '{"account":"${exampleAccount}","to":"13800138000","text":"hello"}'` },
    { title: "本地扇出文字消息", method: "POST", path: "/v1/send", auth: true, code: `curl -X POST ${baseURL}/v1/send \\\n  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>" \\\n  -H "Content-Type: application/json" \\\n  -d '{"account":"${exampleAccount}","group":"family","text":"hello family"}'` },
    { title: "上传并发送图片", method: "POST", path: "/v1/send/media", auth: true, code: `curl -X POST ${baseURL}/v1/send/media \\\n  -H "Authorization: Bearer <CMCC_NOTIFY_AUTH_TOKEN>" \\\n  -F "account=${exampleAccount}" -F "to=13800138000" \\\n  -F "type=IMAGE" -F "caption=photo" -F "file=@./photo.jpg"` },
  ], [baseURL, exampleAccount])
  const copy = async (id: string, value: string) => { await navigator.clipboard.writeText(value); setCopied(id); window.setTimeout(() => setCopied(""), 1600) }
  return <PageFrame eyebrow="Help & Reference" title="API 文档" description="从底层概念、联通测试到应用级生产通知接口的完整说明。">
    <section className="activation-card soft-card"><div className="activation-copy"><div className="section-icon"><QrCode /></div><div><p className="eyebrow">第一步</p><h2>开通中国移动新消息能力</h2><p>使用手机扫描右侧二维码，或直接打开中国移动页面，按照页面提示完成开通。建议使用需要接收消息的中国移动号码操作。</p><a className="activation-link" href={activationURL} target="_blank" rel="noreferrer"><ExternalLink />打开中国移动开通页面</a></div></div><div className="activation-qr"><img src="/cmcc-new-message-activation.svg" alt="中国移动新消息开通二维码" /><span>手机扫码开通</span></div></section>
    <div className="docs-tabs"><SegmentedControl value={tab} options={[{ value: "guide", label: "使用指南", icon: <BookOpen /> }, { value: "concepts", label: "底层概念", icon: <Network /> }, { value: "api", label: "REST API", icon: <KeyRound /> }]} onChange={(value) => setTab(value as "guide" | "concepts" | "api")} /></div>
    {tab === "guide" ? <section className="guide-stack">
      <GuideStep number="1" icon={<KeyRound />} title="添加 CMCC 通道">进入“账户状态” → “CMCC 通道” → “添加通道”。每条通道保存一份 Channel API Key；默认 to 目标可选。状态显示“已连接”后即可发送。API Key 留空表示保留原密钥；修改通道名称会同步关联通知应用。</GuideStep>
      <GuideStep number="2" icon={<UsersRound />} title="创建本地号码组">进入“账户状态” → “本地号码组” → “新建号码组”。每行填写一个 to 目标，也支持逗号或分号分隔，重复值会自动去重。发送时会产生 N 次一对一网关写入。</GuideStep>
      <GuideStep number="3" icon={<BellRing />} title="创建生产通知应用">进入“账户状态” → “通知应用” → “新建应用”，绑定一个 CMCC 通道和固定 to 目标或本地号码组。Token 只显示一次，服务端仅保存哈希；默认禁止调用方改目标，并可设置每分钟请求上限。</GuideStep>
      <GuideStep number="4" icon={<KeyRound />} title="接入业务系统">业务系统使用应用 Token 调用 <code>POST /v1/notify</code>，正文只需提供 <code>title</code> 和 <code>message</code>。不要把 Token 放进 URL、前端代码或日志；泄露时在应用卡片中立即轮换。</GuideStep>
      <GuideStep number="5" icon={<Send />} title="用 WebUI 做联通测试">“发送消息”页面用于验证通道、to 目标、号码组和内容能否写入 CMCC 网关。单个目标留空时可使用通道默认值；号码组会逐个扇出并显示每个目标的写入结果。</GuideStep>
      <GuideStep number="6" icon={<ImageIcon />} title="发送图片、音频、视频或文件">多媒体与文字使用相同的明确 to 路由。文件先上传到 CMCC，再引用媒体 URL 发送；号码组场景只上传一次并逐个目标写入。2026-09-16 已真实验证 PNG 图片与 ZIP 文件送达。</GuideStep>
      <div className="info-note"><Activity />HTTP 202 代表全部目标已写入网关；HTTP 207 代表部分写入失败。两者都不代表终端已经收到或阅读。</div>
    </section> : tab === "concepts" ? <ConceptsPanel /> : <><div className="docs-note"><KeyRound /><div><strong>两类 Bearer Token</strong><p>管理和测试接口使用管理令牌；生产通知入口 <code>/v1/notify</code> 使用独立应用 Token。除 <code>/healthz</code> 外均通过 <code>Authorization</code> 请求头传递，禁止放进 URL。</p></div></div><section className="docs-stack">{examples.map((example, index) => <article className="soft-card api-card" key={`${example.path}-${example.title}-${index}`}><div className="api-heading"><div><span className={cn("method", `method-${example.method.toLowerCase()}`)}>{example.method}</span><h2>{example.title}</h2></div><span className="auth-pill">{example.auth ? <><LockKeyhole />Bearer</> : "公开"}</span></div><code className="endpoint">{example.path}</code><div className="code-block"><pre>{example.code}</pre><button type="button" onClick={() => void copy(`${example.path}-${index}`, example.code)} aria-label={`复制${example.title}示例`}>{copied === `${example.path}-${index}` ? <Check /> : <Copy />}</button></div></article>)}</section><div className="info-note"><Activity />HTTP 202 仅表示全部目标已写入 CMCC 网关；号码组部分失败返回 HTTP 207。当前协议没有可靠的送达或已读回执。</div></>}
  </PageFrame>
}

function ConceptsPanel() {
  return <section className="concepts-stack">
    <div className="concept-flow soft-card" aria-label="CMCC Notify 消息路径"><span>外部系统 / Gotify</span><strong>→</strong><span>CMCC Notify</span><strong>→</strong><span>通道凭据 + to</span><strong>→</strong><span>CMCC 网关</span><strong>→</strong><span>用户终端</span></div>
    <div className="concept-grid">
      <ConceptCard icon={<MessageSquareText />} title="中国移动新消息" badge="底层网络">最终承载文字和多媒体通知的移动消息能力，不等同于 CMCC Notify，也不等同于 OpenClaw。</ConceptCard>
      <ConceptCard icon={<KeyRound />} title="Channel API Key" badge="认证凭据"><code>ak_...</code> 或 <code>app_...</code> 用于认证 CMCC Channel。它不是手机号，也不是管理后台 Token。</ConceptCard>
      <ConceptCard icon={<Radio />} title="CMCC 通道" badge="项目配置">一份命名 Channel 配置，包含 API Key、连接状态、协议地址和可选默认 to。兼容 API 字段仍叫 <code>account</code>。</ConceptCard>
      <ConceptCard icon={<Smartphone />} title="to 目标" badge="实际路由">文字、图片和文件的已验证发送帧都包含 <code>to</code>。它不是备注；但目标授权范围由 CMCC 控制，不能视为任意号码短信接口。</ConceptCard>
      <ConceptCard icon={<UsersRound />} title="本地号码组" badge="本地数据">一组 <code>to</code> 字符串，不是完整接收人实体，也不会在中国移动侧创建群聊。</ConceptCard>
      <ConceptCard icon={<Network />} title="本地扇出" badge="N 次发送">同一通道对号码组成员逐个发送 N 个一对一帧。当前没有证据证明存在 CMCC 原生广播、<code>groupId</code> 或 <code>recipients[]</code>。</ConceptCard>
      <ConceptCard icon={<BellRing />} title="通知应用" badge="调用身份">给 Grafana、NAS、脚本或 Agent 使用的独立 Token，绑定固定通道和固定 to 或号码组。</ConceptCard>
      <ConceptCard icon={<BookOpen />} title="未来 Recipient 模型" badge="尚未实现">未来可以增加“接收人 → 多个通知渠道”的通讯录层；当前号码组成员没有各自的 Channel API Key。</ConceptCard>
    </div>
    <div className="docs-note"><ShieldCheck /><div><strong>证据边界</strong><p>公开指引只确认文字和多媒体；<code>to</code>、上传和媒体帧来自静态审计与真实联调。尚未确认原生群发、任意号码权限、可靠送达回执，以及所有音视频格式兼容性。</p></div></div>
  </section>
}

function ConceptCard({ badge, children, icon, title }: { badge: string; children: ReactNode; icon: ReactNode; title: string }) {
  return <article className="soft-card concept-card"><div className="concept-icon">{icon}</div><div><span>{badge}</span><h2>{title}</h2><p>{children}</p></div></article>
}

function GuideStep({ number, icon, title, children }: { number: string; icon: ReactNode; title: string; children: ReactNode }) {
  return <article className="soft-card guide-step"><div className="guide-number">{number}</div><div className="guide-icon">{icon}</div><div><h2>{title}</h2><p>{children}</p></div></article>
}


function SegmentedControl({ value, options, onChange }: { value: string; options: Array<{ value: string; label: string; icon: ReactNode }>; onChange: (value: string) => void }) { return <div className="mode-switch" style={{ gridTemplateColumns: `repeat(${options.length}, minmax(0, 1fr))` }}>{options.map((option) => <button type="button" key={option.value} className={cn(value === option.value && "active")} onClick={() => onChange(option.value)} aria-pressed={value === option.value}>{option.icon}{option.label}</button>)}</div> }
function AccountRow({ account, compact = false }: { account: AccountStatus; compact?: boolean }) { return <div className={cn("account-row", compact && "compact")}><div className={cn("account-avatar", account.connected && account.enabled ? "online" : "offline")}><Radio /></div><div className="account-main"><strong>{account.name}</strong><span>{account.api_key || "未配置密钥"}</span></div><div className={cn("status-badge", account.connected && account.enabled ? "online" : account.enabled ? "waiting" : "disabled")}><span />{account.connected && account.enabled ? "已连接" : account.enabled ? "未连接" : "已停用"}</div></div> }
function PageFrame({ action, children, description, eyebrow, title }: { action?: ReactNode; children: ReactNode; description: string; eyebrow: string; title: string }) { return <div className="page-frame"><header className="page-header"><div><p className="eyebrow">{eyebrow}</p><h1>{title}</h1><p>{description}</p></div>{action && <div className="page-action">{action}</div>}</header>{children}</div> }
function SectionHeading({ description, icon, title }: { description: string; icon: ReactNode; title: string }) { return <div className="section-heading"><div className="section-icon">{icon}</div><div><h2>{title}</h2><p>{description}</p></div></div> }
function Field({ children, hint, htmlFor, label }: { children: ReactNode; hint?: string; htmlFor: string; label: string }) { return <div className="field"><div className="field-label"><label htmlFor={htmlFor}>{label}</label>{hint && <span>{hint}</span>}</div>{children}</div> }
function EmptyState({ description, icon, title }: { description: string; icon: ReactNode; title: string }) { return <div className="empty-state"><div>{icon}</div><strong>{title}</strong><p>{description}</p></div> }
function BrandMark({ large = false }: { large?: boolean }) { return <div className={cn("brand-mark", large && "large")} aria-hidden="true"><MessageSquareText /><span /></div> }
