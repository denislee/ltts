package main

import (
	"regexp"
	"strings"

	"html"
)

var (
	reScript   = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</\s*script\s*>|<style\b[^>]*>.*?</\s*style\s*>`)
	reTag      = regexp.MustCompile(`(?s)<[^>]+>`)
	reMDImg    = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	reMDLink   = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	reMDCode   = regexp.MustCompile("(?s)```.*?```")
	reMDInline = regexp.MustCompile("`[^`]*`")
	reMDHead   = regexp.MustCompile(`(?m)^#{1,6}\s*`)
	reMDEmph   = regexp.MustCompile(`\*{1,3}([^*]+)\*{1,3}|_{1,3}([^_]+)_{1,3}`)
	reMDRule   = regexp.MustCompile(`(?m)^\s*[-*_]{3,}\s*$`)
	reMDQuote  = regexp.MustCompile(`(?m)^>\s?`)
	reMDList   = regexp.MustCompile(`(?m)^\s*([-*+]|\d+\.)\s+`)
	reWS       = regexp.MustCompile(`[ \t]+`)
	reBlank    = regexp.MustCompile(`\n{3,}`)
)

func extractText(s, ext string) string {
	switch ext {
	case ".html", ".htm", ".xhtml":
		return stripHTML(s)
	case ".md", ".markdown":
		return stripMarkdown(s)
	default:
		return normalize(s)
	}
}

func stripHTML(s string) string {
	s = reScript.ReplaceAllString(s, " ")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return normalize(s)
}

func stripMarkdown(s string) string {
	s = reMDCode.ReplaceAllString(s, " ")
	s = reMDInline.ReplaceAllString(s, " ")
	s = reMDImg.ReplaceAllString(s, " ")
	s = reMDLink.ReplaceAllString(s, "$1")
	s = reMDHead.ReplaceAllString(s, "")
	s = reMDEmph.ReplaceAllString(s, "$1$2")
	s = reMDRule.ReplaceAllString(s, "")
	s = reMDQuote.ReplaceAllString(s, "")
	s = reMDList.ReplaceAllString(s, "")
	return normalize(s)
}

func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(reWS.ReplaceAllString(l, " "))
	}
	s = strings.Join(lines, "\n")
	s = reBlank.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
