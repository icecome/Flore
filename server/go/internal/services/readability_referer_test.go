package services

import "testing"

func TestImageRefererFor(t *testing.T) {
	cases := []struct {
		name     string
		imageURL string
		wantRef  string
		wantOrg  string
	}{
		{
			// 微信图床：命中通用兜底（同站 Referer），不需要特殊域名规则
			name:     "wechat mmbiz generic same-origin",
			imageURL: "http://mmbiz.qpic.cn/mmbiz_gif/xxx/640?wx_fmt=gif",
			wantRef:  "http://mmbiz.qpic.cn",
			wantOrg:  "http://mmbiz.qpic.cn",
		},
		{
			// jintiankansha 中间代理：同样走通用同站兜底
			name:     "jintiankansha proxy generic same-origin",
			imageURL: "http://img2.jintiankansha.me/get?src=http://mmbiz.qpic.cn/mmbiz_gif/xxx/640?wx_fmt=gif",
			wantRef:  "http://img2.jintiankansha.me",
			wantOrg:  "http://img2.jintiankansha.me",
		},
		{
			// pixiv 图床：命中特例规则，使用特定外站 Referer
			name:     "pixiv pximg specific referer",
			imageURL: "https://i.pximg.net/img-original/img/2023/xxx.png",
			wantRef:  "https://www.pixiv.net",
			wantOrg:  "https://www.pixiv.net",
		},
		{
			// 微博图床：命中特例规则
			name:     "weibo sinaimg specific referer",
			imageURL: "https://wx1.sinaimg.cn/large/xxx.jpg",
			wantRef:  "https://weibo.com",
			wantOrg:  "https://weibo.com",
		},
		{
			// 普通图床：通用同站兜底
			name:     "generic cdn same-origin",
			imageURL: "https://cdn.example.com/photo.jpg",
			wantRef:  "https://cdn.example.com",
			wantOrg:  "https://cdn.example.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotRef, gotOrg := imageRefererFor(tc.imageURL)
			if gotRef != tc.wantRef || gotOrg != tc.wantOrg {
				t.Errorf("imageRefererFor(%q) = (%q,%q), want (%q,%q)",
					tc.imageURL, gotRef, gotOrg, tc.wantRef, tc.wantOrg)
			}
		})
	}
}
