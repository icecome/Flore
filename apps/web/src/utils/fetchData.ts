// 通用 fetch 工具：封装请求、错误处理和 toast 提示
// 提取自 useSourcesData / useItemsData 中的重复 fetch 模板

import { getApi } from './api';
import { showToast } from './toast';

interface FetchOptions extends RequestInit {
  baseURL?: string;
  showToast?: boolean;
  toastMessage?: string;
}

/** 发出 GET 请求并解析 JSON，失败时 toast 提示 */
export async function fetchData<T>(
  path: string,
  options: FetchOptions = {}
): Promise<T> {
  const { showToast: show = true, toastMessage, ...fetchOptions } = options;
  const url = path.startsWith('http') ? path : `${getApi()}${path}`;
  const res = await fetch(url, fetchOptions);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return (await res.json()) as T;
}

/** 出错时打印并 toast，返回 undefined */
export function withToast<T>(
  fn: () => Promise<T>,
  toastMsg: string
): Promise<T | undefined> {
  return fn().catch((err) => {
    console.error('Fetch error:', err);
    showToast(toastMsg);
    return undefined as unknown as T;
  });
}
