package faloota

import (
	"context"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/gadelkareem/cachita"
	"github.com/pkg/errors"
	"net/http"
	"sync"
	"time"
)

type Faloota struct {
	*sync.RWMutex
	cache   cachita.Cache
	cancels map[string]context.CancelFunc
	ctxes   map[string]context.Context
}

func NewFaloota() (f *Faloota, err error) {
	f = &Faloota{
		RWMutex: &sync.RWMutex{},
		cache:   cachita.NewMemoryCache(10*time.Minute, 10*time.Minute),
		ctxes:   make(map[string]context.Context),
		cancels: make(map[string]context.CancelFunc),
	}
	_, err = f.Ctx("", "")
	f.Cancel("", "")
	return
}

func (f *Faloota) BypassOnce(inUrl, proxy, userAgent string, verify Action) (cookies []*http.Cookie, err error) {
	cookies, err = f.Bypass(inUrl, proxy, userAgent, verify)
	f.Cancel(proxy, userAgent)
	return
}

func (f *Faloota) Bypass(inUrl, proxy, userAgent string, verify Action) (cookies []*http.Cookie, err error) {
	cacheKey := cachita.Id(inUrl, proxy, userAgent)
	err = f.cache.Get(cacheKey, &cookies)
	if len(cookies) > 0 {
		return cookies, nil
	}

	ctx, err := f.Ctx(proxy, userAgent)
	if err != nil {
		return nil, err
	}

	finalCtx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	err = chromedp.Run(finalCtx, chromedp.Tasks{
		chromedp.Navigate(inUrl),
		verify,
		chromedp.ActionFunc(func(ctxt context.Context, h cdp.Executor) error {
			cks, err := network.GetAllCookies().Do(ctxt, h)
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
		chromedp.Navigate("about:blank"),
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

func (f *Faloota) Ctx(proxy, userAgent string) (ctx context.Context, err error) {
	f.Lock()
	defer f.Unlock()

	k := cachita.Id(proxy, userAgent)
	if ctx, ok := f.ctxes[k]; ok {
		return ctx, nil
	}

	f.ctxes[k], f.cancels[k] = chromedp.NewAllocator(
		context.Background(),
		chromedp.WithExecAllocator(
			//chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-3d-apis", true),
			chromedp.Flag("no-default-browser-check", true),
			chromedp.Flag("no-first-run", true),
			chromedp.Flag("disable-fre", true),
			chromedp.Flag("window-position", "-999999,-999999"),
			chromedp.Flag("window-size", "5,5"),
			chromedp.Flag("user-agent", userAgent),
			chromedp.Flag("proxy-server", proxy),
		),
	)

	chromeCtx := chromedp.FromContext(f.ctxes[k])

	chromeCtx.Browser, err = chromeCtx.Allocator.Allocate(f.ctxes[k])
	if err != nil {
		return nil, errors.Errorf("Error connecting to chrome: %v", err)
	}
	return f.ctxes[k], nil
}

func (f *Faloota) Wait() {
	f.RLock()
	defer f.RUnlock()
	for _, ctx := range f.ctxes {
		chromedp.FromContext(ctx).Allocator.Wait()
	}
}

func (f *Faloota) Close() {
	f.RLock()
	for _, cancel := range f.cancels {
		cancel()
	}
	f.RUnlock()

	f.Wait()
}

func (f *Faloota) Cancel(proxy, useragent string) {
	f.RLock()
	defer f.RUnlock()
	k := cachita.Id(proxy, useragent)
	f.cancels[k]()
	delete(f.cancels, k)
}

type Action interface {
	chromedp.Action
}
