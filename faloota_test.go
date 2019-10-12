package faloota_test

import (
	"github.com/chromedp/chromedp"
	"github.com/gadelkareem/faloota"
	"testing"
)

func TestFaloota_Bypass(t *testing.T) {
	f, err := faloota.NewFaloota()
	if err != nil {
		t.Error(err)
		return
	}
	f.DisableHeadless = true
	verify := chromedp.WaitVisible("#header", chromedp.ByID)
	defer f.Close()
	cookies, err := f.BypassOnce("https://gadelkareem.com", "", "", verify)
	if err != nil {
		t.Error(err)
		return
	}
	if len(cookies) == 0 {
		t.Error("No cookies")
	}
	return
}
