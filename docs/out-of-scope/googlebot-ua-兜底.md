# 不用 Googlebot UA 做抓取兜底

**决定（2026-08-29，用户裁定）：不做，且不接受任何等价变体。**

原方案是全文抓取的第二级重试：主策略（Chrome UA + HTTP/2 + Referer）失败时，换
`Googlebot` UA 重跑一次。2026-08-29 的 27 个失败 URL × 6 策略实测里，它是唯一救回
`analyticsindiamag.com` 的策略——收益真实，但只有这一个站点。

## 为什么否决

实现过程中发现：**本机杀软把任何含 `Googlebot` 字面量的二进制判为 `TrojanSpy/Agent.db`
并直接删除**——伪装成爬虫的 UA 是它的启发式特征之一。代价是构建产物被杀，换回来的是
一个站点。性价比不成立。

**同时否决"绕开特征"的做法**：运行时拼字符串、编码后解码、放进配置文件由用户填写，
都属于为躲避本机安全软件而隐藏程序特征，不做。

## 因此

抓取只跑一趟，用实测有效的能力包（Chrome 141 UA + HTTP/2 + `Referer`）。
`analyticsindiamag.com` 移出验收范围，落到失败分类展示 + 「在浏览器中打开原文」。

追溯：Epic `mrrss-c58513`（`forge tracker show mrrss-c58513`）；原 spec D3 见
`git show fe3c371b:docs/tracker/全文抓取增强/spec.md`。
