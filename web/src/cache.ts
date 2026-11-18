// 页面数据的内存缓存：切回一个页面时先显示上次的数据，同时在后台重新取，取到再替换。
// 这样切换页签时不会先空一下、再“啪”地出现，布局也不会跳。
// 只存在内存里（刷新页面就没了），不进浏览器存储；退出登录时清空。
const mem = new Map<string, unknown>();

/** 读上次缓存的值。 */
export function cached<T>(key: string): T | undefined {
  return mem.get(key) as T | undefined;
}

/** 记下最新的值并原样返回，方便写成 api.x().then(v => set(remember(key, v)))。 */
export function remember<T>(key: string, value: T): T {
  mem.set(key, value);
  return value;
}

/** 清空全部缓存（退出登录、切换账号时）。 */
export function clearCache() {
  mem.clear();
}
