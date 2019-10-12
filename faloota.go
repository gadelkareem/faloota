package faloota

import (
	"context"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/gadelkareem/cachita"
	"github.com/gadelkareem/go-helpers"
	"github.com/pkg/errors"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Faloota struct {
	*sync.Mutex
	cache           cachita.Cache
	cancels         map[string]context.CancelFunc
	ctxes           map[string]context.Context
	proxyAuthRegex  *regexp.Regexp
	DisableHeadless bool
}

func NewFaloota() (f *Faloota, err error) {
	f = &Faloota{
		Mutex:          &sync.Mutex{},
		cache:          cachita.NewMemoryCache(1*time.Hour, 1*time.Hour),
		ctxes:          make(map[string]context.Context),
		cancels:        make(map[string]context.CancelFunc),
		proxyAuthRegex: regexp.MustCompile(`[^/:]+:[^/:@]+@`),
	}
	_, err = f.Ctx("", "")
	f.Cancel("", "")
	return
}

func (f *Faloota) BypassOnce(inUrl, proxy, userAgent string, verify Action) (cookies []*http.Cookie, err error) {
	id := h.RandomString(5)
	cookies, err = f.Bypass(inUrl, proxy, userAgent, verify, id)
	f.Cancel(proxy, userAgent, id)
	return
}

func (f *Faloota) Bypass(inUrl, proxy, userAgent string, verify Action, id ...string) (cookies []*http.Cookie, err error) {
	cacheKey := key(inUrl, proxy, userAgent)
	err = f.cache.Get(cacheKey, &cookies)
	if len(cookies) > 0 {
		return cookies, nil
	}

	ctx, err := f.Ctx(proxy, userAgent, id...)
	if err != nil {
		return nil, err
	}

	finalCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	err = chromedp.Run(finalCtx, chromedp.Tasks{
		chromedp.Navigate(inUrl),
		verify,
		chromedp.ActionFunc(func(ctxt context.Context) error {
			cks, err := network.GetAllCookies().Do(ctxt)
			if err != nil {
				return err
			}
			for _, ck := range cks {
				cookies = append(cookies, &http.Cookie{
					Name:     ck.Name,
					Value:    ck.Value,
					Path:     ck.Path,
					Domain:   ck.Domain,
					Expires:  time.Unix(int64(ck.Expires), 0),
					HttpOnly: ck.HTTPOnly,
					Secure:   ck.Secure,
				})
			}
			return nil
		}),
	})
	if err != nil {
		return nil, err
	}

	if len(cookies) == 0 {
		return nil, errors.Errorf("URL %s Failed", inUrl)
	}

	err = f.cache.Put(cacheKey, cookies, 0)
	return cookies, err
}

func (f *Faloota) Ctx(proxy, userAgent string, id ...string) (ctx context.Context, err error) {
	f.Lock()
	defer f.Unlock()
	k := key(proxy, userAgent, id...)

	if ctx, ok := f.ctxes[k]; ok {
		return ctx, nil
	}

	if strings.Contains(proxy, "@") {
		// No proxy auth support https://bugs.chromium.org/p/chromium/issues/detail?id=615947
		proxy = f.proxyAuthRegex.ReplaceAllString(proxy, "")
	}
	opts := []chromedp.ExecAllocatorOption{
		chromedp.DisableGPU,
		chromedp.NoDefaultBrowserCheck,
		chromedp.NoFirstRun,
		chromedp.UserAgent(userAgent),
		chromedp.ProxyServer(proxy),
		chromedp.WindowSize(5, 5),
		chromedp.Flag("disable-3d-apis", true),
		chromedp.Flag("disable-fre", true),
		chromedp.Flag("disable-notifications", true),
		chromedp.Flag("window-position", "-999999,-999999"),
		// After Puppeteer's default behavior.
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("enable-features", "NetworkService,NetworkServiceInProcess"),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-features", "site-per-process,TranslateUI,BlinkGenPropertyTrees"),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-ipc-flooding-protection", true),
		chromedp.Flag("disable-popup-blocking", true),
		chromedp.Flag("disable-prompt-on-repost", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("force-color-profile", "srgb"),
		chromedp.Flag("metrics-recording-only", true),
		chromedp.Flag("safebrowsing-disable-auto-update", true),
		chromedp.Flag("enable-automation", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("use-mock-keychain", true),
	}
	if !f.DisableHeadless {
		opts = append(opts, chromedp.Headless, )
	}
	ctx, _ = chromedp.NewExecAllocator(
		context.Background(),
		opts...,
	)
	f.ctxes[k], f.cancels[k] = chromedp.NewContext(ctx)

	return f.ctxes[k], nil
}

func (f *Faloota) Wait() {
	f.Lock()
	defer f.Unlock()
	for k, ctx := range f.ctxes {
		chromedp.FromContext(ctx).Allocator.Wait()
		delete(f.ctxes, k)
	}
}

func (f *Faloota) Close() {
	f.Lock()
	for k, cancel := range f.cancels {
		cancel()
		delete(f.cancels, k)
	}
	f.Unlock()

	f.Wait()
}

func (f *Faloota) Cancel(proxy, useragent string, id ...string) {
	f.Lock()
	defer f.Unlock()
	k := key(proxy, useragent, id...)
	if cancel, ok := f.cancels[k]; ok {
		cancel()
		delete(f.ctxes, k)
		delete(f.cancels, k)
	}
}

func key(proxy, userAgent string, id ...string) string {
	return cachita.Id(proxy, userAgent, cachita.Id(id...))
}

type Action interface {
	chromedp.Action
}
