package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type googleEngine struct{}

func (g *googleEngine) ID() string   { return "google" }
func (g *googleEngine) Name() string { return "Google Translate TTS (online)" }
func (g *googleEngine) Description() string {
	return "Free Google translate_tts endpoint. No install, requires network. Quality: decent."
}
func (g *googleEngine) Ready() bool                                         { return true }
func (g *googleEngine) Install(context.Context, func(string)) error         { return nil }
func (g *googleEngine) ChunkLimit() int                                     { return 180 }
func (g *googleEngine) Format() string                                      { return "mp3" }

func (g *googleEngine) Voices() []Voice {
	return []Voice{
		{ID: "en", Label: "English",   Lang: "en"},
		{ID: "en-US", Label: "English (US)", Lang: "en-US"},
		{ID: "en-GB", Label: "English (UK)", Lang: "en-GB"},
		{ID: "es", Label: "Spanish",   Lang: "es"},
		{ID: "pt", Label: "Portuguese", Lang: "pt"},
		{ID: "pt-BR", Label: "Portuguese (Brazil)", Lang: "pt-BR"},
		{ID: "fr", Label: "French",    Lang: "fr"},
		{ID: "de", Label: "German",    Lang: "de"},
		{ID: "it", Label: "Italian",   Lang: "it"},
		{ID: "nl", Label: "Dutch",     Lang: "nl"},
		{ID: "ja", Label: "Japanese",  Lang: "ja"},
		{ID: "zh-CN", Label: "Chinese", Lang: "zh-CN"},
	}
}

func (g *googleEngine) Synth(ctx context.Context, text string, voice Voice, slow bool, dst string) error {
	q := url.Values{}
	q.Set("ie", "UTF-8")
	q.Set("q", text)
	q.Set("tl", voice.ID)
	q.Set("client", "tw-ob")
	q.Set("total", "1")
	q.Set("idx", "0")
	q.Set("textlen", fmt.Sprint(len(text)))
	if slow {
		q.Set("ttsspeed", "0.24")
	} else {
		q.Set("ttsspeed", "1")
	}

	u := "https://translate.google.com/translate_tts?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) Gecko/20100101 Firefox/128.0")
	req.Header.Set("Referer", "https://translate.google.com/")

	client := &http.Client{Timeout: 30 * time.Second}
	var lastErr error
	for attempt := range [3]struct{}{} {
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
			continue
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
			continue
		}
		out, err := os.Create(dst)
		if err != nil {
			resp.Body.Close()
			return err
		}
		_, err = io.Copy(out, resp.Body)
		resp.Body.Close()
		out.Close()
		if err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return fmt.Errorf("google tts: %w", lastErr)
}

// splitText breaks s into chunks of at most max runes, preferring to cut at
// sentence, clause, then word boundaries.
func splitText(s string, max int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for len(s) > 0 {
		if len(s) <= max {
			out = append(out, strings.TrimSpace(s))
			break
		}
		cut := findCut(s, max)
		piece := strings.TrimSpace(s[:cut])
		if piece != "" {
			out = append(out, piece)
		}
		s = strings.TrimSpace(s[cut:])
	}
	return out
}

func findCut(s string, max int) int {
	if len(s) <= max {
		return len(s)
	}
	window := s[:max]
	for _, seps := range []string{".!?\n", ",;:", " \t"} {
		if i := strings.LastIndexAny(window, seps); i > 0 {
			return i + 1
		}
	}
	return max
}
