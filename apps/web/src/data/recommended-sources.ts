export interface RecommendedSource {
  name: string;
  url: string;
  hint: string;
}

// 内置精选源列表（T-v02-03）：给没有 OPML 文件的纯新手一条活路。
// 均为常用且公开可订阅的中文 RSS 源。若个别源已失效，可在此更新。
export const RECOMMENDED_SOURCES: RecommendedSource[] = [
  { name: '少数派', url: 'https://sspai.com/feed', hint: '效率工具与数字生活' },
  { name: '爱范儿', url: 'https://www.ifanr.com/feed', hint: '科技与消费电子' },
  { name: '36氪', url: 'https://36kr.com/feed', hint: '创投与商业' },
  { name: '极客公园', url: 'https://www.geekpark.net/rss', hint: '科技与创新' },
  { name: '阮一峰的网络日志', url: 'https://www.ruanyifeng.com/blog/atom.xml', hint: '技术随笔与周刊' },
  { name: '酷壳 CoolShell', url: 'https://coolshell.cn/feed', hint: '程序员技术博客' },
  { name: 'Hi-iD', url: 'https://hi-id.com/feed', hint: '设计观察' },
  { name: '月光博客', url: 'https://www.williamlong.info/rss.xml', hint: '互联网评论' },
  { name: '月光下的狼', url: 'https://www.mgpyh.com/feed', hint: '技术与生活' },
];