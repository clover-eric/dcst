package task

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/XIU2/CloudflareSpeedTest/utils"
)

const (
	bufferSize                     = 256 * 1024
	defaultURL                     = "https://cf.xiu2.xyz/url"
	defaultTimeout                 = 10 * time.Second
	defaultWarmup                  = 1 * time.Second
	defaultStreams                 = 16
	maxStreams                     = 64
	defaultDisableDownload         = false
	defaultTestNum                 = 20
	defaultMinSpeed        float64 = 0.0
)

var (
	URL     = defaultURL
	Timeout = defaultTimeout
	Warmup  = defaultWarmup
	Disable = defaultDisableDownload
	Streams = defaultStreams

	TestCount = defaultTestNum
	MinSpeed  = defaultMinSpeed
)

func checkDownloadDefault() {
	if URL == "" {
		URL = defaultURL
	}
	if Timeout <= 0 {
		Timeout = defaultTimeout
	}
	if Warmup < 0 {
		Warmup = 0
	}
	if Warmup >= Timeout {
		Warmup = Timeout / 5
	}
	if Streams < 1 {
		Streams = defaultStreams
	}
	if Streams > maxStreams {
		Streams = maxStreams
	}
	if TestCount <= 0 {
		TestCount = defaultTestNum
	}
	if MinSpeed <= 0.0 {
		MinSpeed = defaultMinSpeed
	}
}

func TestDownloadSpeed(ipSet utils.PingDelaySet) (speedSet utils.DownloadSpeedSet) {
	checkDownloadDefault()
	if Disable {
		return utils.DownloadSpeedSet(ipSet)
	}
	if len(ipSet) <= 0 { // IP 数组长度(IP数量) 大于 0 时才会继续下载测速
		utils.Yellow.Println("[信息] 延迟测速结果 IP 数量为 0，跳过下载测速。")
		return
	}
	testNum := TestCount                        // 等待下载测速的队列数量 先默认等于 下载测速数量(-dn）
	if len(ipSet) < TestCount || MinSpeed > 0 { // 如果延迟测速并过滤后的 IP 数组长度(IP数量) 小于 下载测速数量(-dn），（即 -dn 预期数量是不够的），或者指定了 下载测速下限 (-sl) 条件（这就可能要全部下载测速一遍，直到找齐预期数量或测完为止），则 等待下载测速的队列数量 修正为 IP 数量
		testNum = len(ipSet)
	}
	if testNum < TestCount { // 如果 等待下载测速的队列数量 小于 下载测速数量(-dn），（显然 -dn 预期数量是不够的），所以 下载测速数量(-dn）修正为 等待下载测速的队列数量
		TestCount = testNum
	}

	utils.Cyan.Printf("开始下载测速（下限：%.2f MB/s, 数量：%d, 队列：%d, 并发：%d, 预热：%v）\n",
		MinSpeed, TestCount, testNum, Streams, Warmup)
	// 控制 下载测速进度条 与 延迟测速进度条 长度一致（强迫症）
	bar_a := len(strconv.Itoa(len(ipSet)))
	bar_b := "     "
	for i := 0; i < bar_a; i++ {
		bar_b += " "
	}
	bar := utils.NewBar(TestCount, bar_b, "")
	for i := 0; i < testNum; i++ {
		speed, colo := downloadHandler(ipSet[i].IP)
		ipSet[i].DownloadSpeed = speed
		if ipSet[i].Colo == "" { // 只有当 Colo 是空的时候，才写入，否则代表之前是 httping 测速并获取过了
			ipSet[i].Colo = colo
		}
		// 在每个 IP 下载测速后，以 [下载速度下限] 条件过滤结果
		if speed >= MinSpeed*1024*1024 {
			bar.Grow(1, "")
			speedSet = append(speedSet, ipSet[i]) // 高于下载速度下限时，添加到新数组中
			if len(speedSet) == TestCount {       // 凑够满足条件的 IP 时（下载测速数量 -dn），就跳出循环
				break
			}
		}
	}
	bar.Done()
	if MinSpeed == 0.00 { // 如果没有指定下载速度下限，则直接返回所有测速数据
		speedSet = utils.DownloadSpeedSet(ipSet)
	} else if utils.Debug && len(speedSet) == 0 { // 如果指定了下载速度下限，且是调试模式下，且没有找到任何一个满足条件的 IP 时，返回所有测速数据，供用户查看当前的测速结果，以便适当调低预期测速条件
		utils.Yellow.Println("[调试] 没有满足 下载速度下限 条件的 IP，忽略条件返回所有测速数据（方便下次测速时调整条件）。")
		speedSet = utils.DownloadSpeedSet(ipSet)
	}
	// 按速度排序
	sort.Sort(speedSet)
	return
}

func getDialContext(ip *net.IPAddr) func(ctx context.Context, network, address string) (net.Conn, error) {
	var fakeSourceAddr string
	if isIPv4(ip.String()) {
		fakeSourceAddr = fmt.Sprintf("%s:%d", ip.String(), TCPPort)
	} else {
		fakeSourceAddr = fmt.Sprintf("[%s]:%d", ip.String(), TCPPort)
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, fakeSourceAddr)
	}
}

// 统一的请求报错调试输出
func printDownloadDebugInfo(ip *net.IPAddr, err error, statusCode int, url, lastRedirectURL string, response *http.Response) {
	finalURL := url // 默认的最终 URL，这样当 response 为空时也能输出
	if lastRedirectURL != "" {
		finalURL = lastRedirectURL // 如果 lastRedirectURL 不是空，说明重定向过，优先输出最后一次要重定向至的目标
	} else if response != nil && response.Request != nil && response.Request.URL != nil {
		finalURL = response.Request.URL.String() // 如果 response 不为 nil，且 Request 和 URL 都不为 nil，则获取最后一次成功的响应地址
	}
	if url != finalURL { // 如果 URL 和最终地址不一致，说明有重定向，是该重定向后的地址引起的错误
		if statusCode > 0 { // 如果状态码大于 0，说明是后续 HTTP 状态码引起的错误
			utils.Red.Printf("[调试] IP: %s, 下载测速终止，HTTP 状态码: %d, 下载测速地址: %s, 出错的重定向后地址: %s\n", ip.String(), statusCode, url, finalURL)
		} else {
			utils.Red.Printf("[调试] IP: %s, 下载测速失败，错误信息: %v, 下载测速地址: %s, 出错的重定向后地址: %s\n", ip.String(), err, url, finalURL)
		}
	} else { // 如果 URL 和最终地址一致，说明没有重定向
		if statusCode > 0 { // 如果状态码大于 0，说明是后续 HTTP 状态码引起的错误
			utils.Red.Printf("[调试] IP: %s, 下载测速终止，HTTP 状态码: %d, 下载测速地址: %s\n", ip.String(), statusCode, url)
		} else {
			utils.Red.Printf("[调试] IP: %s, 下载测速失败，错误信息: %v, 下载测速地址: %s\n", ip.String(), err, url)
		}
	}
}

// downloadStream downloads from one independent TCP/TLS connection. Multiple
// streams are intentional: a single TCP connection is commonly shaped below
// the capacity of a fast access link.
func downloadStream(ctx context.Context, ip *net.IPAddr) (int64, string, error) {
	tlsTimeout := 5 * time.Second
	if Timeout < tlsTimeout {
		tlsTimeout = Timeout
	}
	transport := &http.Transport{
		DialContext: getDialContext(ip),
		// Keep each stream on its own HTTP/1.1 connection. Independent TCP
		// connections avoid HTTP/2 flow-control heads-of-line blocking and
		// remain compatible with public endpoints that reject HTTP/2 clients.
		ForceAttemptHTTP2:     false,
		DisableCompression:    true,
		TLSHandshakeTimeout:   tlsTimeout,
		ResponseHeaderTimeout: Timeout,
		WriteBufferSize:       64 * 1024,
		MaxIdleConns:          1,
		MaxIdleConnsPerHost:   1,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return http.ErrUseLastResponse
			}
			if req.Header.Get("Referer") == defaultURL {
				req.Header.Del("Referer")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, URL, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/98.0.4758.80 Safari/537.36")
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, getHeaderColo(resp.Header), fmt.Errorf("HTTP 状态码 %d", resp.StatusCode)
	}

	colo := getHeaderColo(resp.Header)
	buffer := make([]byte, bufferSize)
	var received int64
	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			received += int64(n)
		}
		if readErr != nil {
			if readErr == io.EOF {
				return received, colo, nil
			}
			return received, colo, readErr
		}
	}
}

// downloadHandler measures aggregate steady-state throughput over independent
// streams after a configurable TCP/TLS warm-up window.
func downloadHandler(ip *net.IPAddr) (float64, string) {
	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	var (
		received  atomic.Int64
		coloMu    sync.Mutex
		colo      string
		errOnce   sync.Once
		firstErr  error
		wg        sync.WaitGroup
		startTime = time.Now()
	)
	for i := 0; i < Streams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, streamColo, err := downloadStream(ctx, ip)
			if n > 0 {
				received.Add(n)
			}
			if streamColo != "" {
				coloMu.Lock()
				if colo == "" {
					colo = streamColo
				}
				coloMu.Unlock()
			}
			if err != nil && ctx.Err() == nil {
				errOnce.Do(func() { firstErr = err })
			}
		}()
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()

	if Warmup > 0 {
		timer := time.NewTimer(Warmup)
		select {
		case <-finished:
			timer.Stop()
			total := received.Load()
			elapsed := time.Since(startTime).Seconds()
			if utils.Debug && firstErr != nil {
				utils.Red.Printf("[调试] IP: %s, 下载流提前结束，错误信息: %v\n", ip.String(), firstErr)
			}
			coloMu.Lock()
			defer coloMu.Unlock()
			if total > 0 && elapsed > 0 {
				return float64(total) / elapsed, colo
			}
			return 0, colo
		case <-timer.C:
		}
	}

	measureStartTime := time.Now()
	measureStartBytes := received.Load()
	<-finished
	measureElapsed := time.Since(measureStartTime).Seconds()
	measureBytes := received.Load() - measureStartBytes
	coloMu.Lock()
	defer coloMu.Unlock()

	if utils.Debug && firstErr != nil && measureBytes == 0 {
		utils.Red.Printf("[调试] IP: %s, 下载测速失败，错误信息: %v, 下载测速地址: %s\n", ip.String(), firstErr, URL)
	}
	if measureBytes <= 0 || measureElapsed <= 0 {
		return 0, colo
	}
	return float64(measureBytes) / measureElapsed, colo
}
