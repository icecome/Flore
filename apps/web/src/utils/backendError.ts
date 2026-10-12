/** 后端错误文案的中文化。
 *
 * 后端 handlers 的 safeError() 会原样透传已知业务错误字符串，而这些字符串是英文的
 * （例如 `file provider not configured`）。前端若直接把 err.message 弹给用户，
 * 中文界面就会出现英文。这里做一次映射，未命中时保留原文，不吞掉任何信息。
 *
 * 新增后端错误时同步补这里；映射表只覆盖会到达用户界面的错误。
 */

const BACKEND_ERROR_MESSAGES: { match: RegExp; text: string }[] = [
  // 同步配置与连通性
  { match: /sync api endpoint is empty/i, text: '请先填写服务端地址' },
  { match: /sync api token is empty/i, text: '请先填写访问令牌' },
  { match: /sync api username or password is empty/i, text: '请填写用户名和密码' },
  { match: /unsupported api provider/i, text: '不支持的服务端类型' },
  { match: /unsupported file provider/i, text: '不支持的存储类型' },
  { match: /file provider not configured/i, text: '请先选择存储类型' },
  { match: /file provider .* not supported/i, text: '该存储类型暂不支持' },
  { match: /webdav url is empty/i, text: '请填写 WebDAV 地址' },
  { match: /webdav invalid url/i, text: 'WebDAV 地址格式不正确' },
  { match: /webdav unreachable/i, text: '无法连接 WebDAV 服务，请检查地址与网络' },
  { match: /webdav authentication failed/i, text: 'WebDAV 认证失败，请检查用户名和密码' },
  { match: /webdav probe failed \(http (\d+)\)/i, text: 'WebDAV 服务返回错误（HTTP $1）' },
  { match: /miniflux authentication failed/i, text: '认证失败，请检查用户名和密码' },
  { match: /authentication failed/i, text: '认证失败，请检查地址与凭证' },
  { match: /authentication required/i, text: '需要登录，请检查凭证' },
  { match: /unreachable/i, text: '无法连接到服务端，请检查地址与网络' },
  { match: /blocked connection|private IP|loopback/i, text: '目标地址不允许访问（本机或内网地址已被阻止）' },

  // 订阅源
  { match: /already exists/i, text: '该订阅源已存在' },
  { match: /invalid url/i, text: '地址格式不正确' },
  { match: /source not found/i, text: '订阅源不存在' },

  // 备份与数据库
  { match: /backup .* not found/i, text: '备份文件不存在' },
  { match: /invalid backup file/i, text: '备份文件无效或已损坏' },
  { match: /not a valid sqlite/i, text: '文件不是有效的数据库备份' },

  // 通用
  { match: /internal_server_error/i, text: '服务处理失败，请稍后重试' },
  { match: /^HTTP \d+$/i, text: '服务请求失败，请稍后重试' },
  { match: /rate limit exceeded/i, text: '操作过于频繁，请稍后再试' },
  { match: /unauthorized/i, text: '会话未授权，请重启应用' },
];

/** 把后端返回的英文错误转成中文提示。未命中映射时原样返回。 */
export function translateBackendError(message: string | undefined | null): string | null {
  if (!message) return null;
  for (const entry of BACKEND_ERROR_MESSAGES) {
    if (entry.match.test(message)) {
      return message.replace(entry.match, entry.text);
    }
  }
  return message;
}
