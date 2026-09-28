package fulltext

// OutcomeNoLink means the article has no absolute http(s) link to fetch; the
// fetch was never attempted.
const OutcomeNoLink Outcome = "no_link"

// Message is the Chinese reason shown to the reader for a failed fetch
// (spec D11), empty on success. Result.Detail stays in the log.
func (o Outcome) Message() string {
	switch o {
	case OutcomeSuccess:
		return ""
	case OutcomeNoContent:
		return "这一页没有可提取的正文，可能是视频、PDF，或需要脚本渲染的页面。"
	case OutcomeBlocked:
		return "站点拒绝了这次抓取，可以在浏览器里打开原文。"
	case OutcomeParseFailed:
		return "页面取回来了，但没能从中提取出正文。"
	case OutcomeUnreachable:
		return "连不上原文站点，请检查网络或代理设置。"
	case OutcomeNoLink:
		return "这篇文章没有可抓取的原文链接。"
	default:
		return "未能获取全文。"
	}
}
