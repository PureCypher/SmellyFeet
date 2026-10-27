package server

import (
	"strings"
	"testing"
)

const testPageURL = "https://example.com/news/story"

func TestSanitizeDropsScriptSubtree(t *testing.T) {
	out := sanitizeArticleHTML(`<p>Before</p><script>alert("x")</script><p>After</p>`, testPageURL)
	if strings.Contains(out, "alert") {
		t.Fatalf("script content leaked: %s", out)
	}
	if !strings.Contains(out, "<p>Before</p>") || !strings.Contains(out, "<p>After</p>") {
		t.Fatalf("expected surrounding paragraphs kept: %s", out)
	}
}

func TestSanitizeDropsIframeAndStyle(t *testing.T) {
	out := sanitizeArticleHTML(`<iframe src="//evil.example"></iframe><style>body{display:none}</style><p>ok</p>`, testPageURL)
	if strings.Contains(out, "iframe") || strings.Contains(out, "evil.example") || strings.Contains(out, "style") {
		t.Fatalf("widget/metadata leaked: %s", out)
	}
}

func TestSanitizeNeutralizesJavascriptHref(t *testing.T) {
	out := sanitizeArticleHTML(`<a href="javascript:alert(1)">click</a>`, testPageURL)
	if strings.Contains(strings.ToLower(out), "javascript:") {
		t.Fatalf("javascript: href survived: %s", out)
	}
	if !strings.Contains(out, "click") {
		t.Fatalf("anchor text should survive: %s", out)
	}
}

func TestSanitizeKeepsHTTPSLinkWithNoopener(t *testing.T) {
	out := sanitizeArticleHTML(`<a href="/related">more</a>`, testPageURL)
	if !strings.Contains(out, `href="https://example.com/related"`) {
		t.Fatalf("relative link not resolved: %s", out)
	}
	if !strings.Contains(out, "noopener") || !strings.Contains(out, "nofollow") {
		t.Fatalf("external-link hardening missing: %s", out)
	}
}

func TestSanitizeRewritesImageSrcToProxy(t *testing.T) {
	out := sanitizeArticleHTML(`<img src="/img/hero.png" alt="Hero">`, testPageURL)
	if !strings.Contains(out, `src="/img?u=`) {
		t.Fatalf("image src not proxied: %s", out)
	}
	if !strings.Contains(out, `alt="Hero"`) {
		t.Fatalf("alt attribute lost: %s", out)
	}
	if !strings.Contains(out, "loading=\"lazy\"") {
		t.Fatalf("lazy-loading missing: %s", out)
	}
}

func TestSanitizeDropsDataURIImage(t *testing.T) {
	out := sanitizeArticleHTML(`<img src="data:image/png;base64,iVBOR">`, testPageURL)
	if strings.Contains(out, "<img") {
		t.Fatalf("data: image survived: %s", out)
	}
}

func TestSanitizeDropsImgWithoutUsableSrc(t *testing.T) {
	out := sanitizeArticleHTML(`<p>Text</p><img alt="nothing">`, testPageURL)
	if strings.Contains(out, "<img") {
		t.Fatalf("srcless img survived: %s", out)
	}
	if !strings.Contains(out, "Text") {
		t.Fatalf("paragraph lost: %s", out)
	}
}

func TestSanitizeStripsEventHandlersAndUnknownAttrs(t *testing.T) {
	out := sanitizeArticleHTML(`<p onclick="evil()" data-track="x" style="color:red">Body</p>`, testPageURL)
	for _, bad := range []string{"onclick", "data-track", "style"} {
		if strings.Contains(out, bad) {
			t.Fatalf("attribute %s survived: %s", bad, out)
		}
	}
	if !strings.Contains(out, "Body") {
		t.Fatalf("paragraph lost: %s", out)
	}
}

func TestSanitizeUnwrapsChromeKeepingChildren(t *testing.T) {
	out := sanitizeArticleHTML(`<div class="entry"><section><h2>Heading</h2><span>Inner</span></section></div>`, testPageURL)
	if strings.Contains(out, "<div") || strings.Contains(out, "<section") || strings.Contains(out, "<span") {
		t.Fatalf("chrome tags survived: %s", out)
	}
	for _, want := range []string{"<h2>Heading</h2>", "Inner"} {
		if !strings.Contains(out, want) {
			t.Fatalf("children lost (missing %s): %s", want, out)
		}
	}
}

func TestSanitizeReEscapesHostileText(t *testing.T) {
	out := sanitizeArticleHTML(`<p>1 &lt; 2 &amp; "quoted" &lt;/p&gt;&lt;script&gt;</p>`, testPageURL)
	if !strings.Contains(out, "1 &lt; 2") {
		t.Fatalf("entity text lost: %s", out)
	}
	if strings.Contains(out, "<script>") {
		t.Fatalf("escaped script marker became real markup: %s", out)
	}
}

func TestSanitizeDropsNonDigitDimensions(t *testing.T) {
	out := sanitizeArticleHTML(`<img src="/a.png" width="100%" height="auto">`, testPageURL)
	if strings.Contains(out, `width="100%"`) || strings.Contains(out, `height="auto"`) {
		t.Fatalf("non-digit dimensions survived: %s", out)
	}
}

func TestSanitizeKeepsMailtoLink(t *testing.T) {
	out := sanitizeArticleHTML(`<a href="mailto:tips@example.com">tips</a>`, testPageURL)
	if !strings.Contains(out, `href="mailto:tips@example.com"`) {
		t.Fatalf("mailto link lost: %s", out)
	}
}

func TestSanitizeMalformedInputDoesNotPanicOrEmitRaw(t *testing.T) {
	for _, raw := range []string{
		`<p><b>unclosed`,
		`<img src="https://example.com/x.png`,
		`<a href="https://example.com">unclosed`,
		`</p></div></table>stray closers`,
		`<div><ul><li>one<li>two</ul>`,
		`<p>&#60;script&#62;entity&#60;/script&#62;</p>`,
		`<a href="jAvAsCrIpT:alert(1)">mixed case scheme</a>`,
		`<img src="hTtPs://example.com/i.png">`,
	} {
		out := sanitizeArticleHTML(raw, testPageURL) // panics fail the test
		for _, bad := range []string{"<script", "javascript:"} {
			if strings.Contains(strings.ToLower(out), bad) {
				t.Fatalf("malformed input %q produced %q", raw, out)
			}
		}
	}
}

func TestSanitizeBodyNilAndEmpty(t *testing.T) {
	empty := ""
	if sanitizeBody(nil, testPageURL) != "" {
		t.Fatalf("nil input must render empty")
	}
	if sanitizeBody(&empty, testPageURL) != "" {
		t.Fatalf("empty input must render empty")
	}
	out := string(sanitizeBody(ptr("<p>ok</p>"), testPageURL))
	if !strings.Contains(out, "<p>ok</p>") {
		t.Fatalf("content lost: %s", out)
	}
}

func ptr(s string) *string { return &s }

func TestSanitizeDemotesBodyH1(t *testing.T) {
	out := sanitizeArticleHTML(`<h1>Section Headline</h1><p>Body text.</p>`, testPageURL)
	if strings.Contains(out, "<h1>") {
		t.Fatalf("body h1 survived, breaking heading hierarchy: %s", out)
	}
	if !strings.Contains(out, "<h2>Section Headline</h2>") {
		t.Fatalf("demoted h2 missing: %s", out)
	}
}
