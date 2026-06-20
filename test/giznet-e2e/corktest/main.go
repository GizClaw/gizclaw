//go:build manual

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GizClaw/gizclaw-go/pkg/giznet"
)

const defaultServiceID uint64 = 57

type allowAllPolicy struct{}

func (allowAllPolicy) AllowPeer(giznet.PublicKey) bool {
	return true
}

func (allowAllPolicy) AllowService(giznet.PublicKey, uint64) bool {
	return true
}

type config struct {
	mode               string
	duration           time.Duration
	streams            int
	chunkSize          int
	corkEvery          int
	corkDuration       time.Duration
	writeInterval      time.Duration
	disconnectWait     time.Duration
	reportInterval     time.Duration
	readTimeout        time.Duration
	forceGCInterval    time.Duration
	freeOSMemInterval  time.Duration
	maxHeapGrowthMB    int64
	maxHeapSysGrowthMB int64
	serviceID          uint64
	cipherMode         giznet.CipherMode
	requireOffline     bool
	pprofAddr          string
	memCSV             string
}

type counters struct {
	clientToServerBytes atomic.Int64
	serverToClientBytes atomic.Int64
	clientToServerReads atomic.Int64
	serverToClientReads atomic.Int64
}

type streamPair struct {
	id     int
	client net.Conn
	server net.Conn
}

type memSnapshot struct {
	heapAlloc    uint64
	heapInuse    uint64
	heapIdle     uint64
	heapReleased uint64
	heapSys      uint64
	objects      uint64
	totalAlloc   uint64
	mallocs      uint64
	frees        uint64
	numGC        uint32
	goroutines   int
}

type endpointPair struct {
	serverKey  *giznet.KeyPair
	clientKey  *giznet.KeyPair
	server     *giznet.Listener
	client     *giznet.Listener
	serverConn *giznet.Conn
	clientConn *giznet.Conn
}

func main() {
	cfg := parseFlags()
	if err := run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "corktest failed: %v\n", err)
		os.Exit(1)
	}
}

func parseFlags() config {
	var cipherMode string
	cfg := config{}
	flag.StringVar(&cfg.mode, "mode", "trickle", "scenario: trickle, disconnect, all")
	flag.DurationVar(&cfg.duration, "duration", 60*time.Second, "trickle transfer duration")
	flag.IntVar(&cfg.streams, "streams", 4, "concurrent KCP streams")
	flag.IntVar(&cfg.chunkSize, "chunk", 8*1024, "write chunk size in bytes")
	flag.IntVar(&cfg.corkEvery, "cork-every", 128, "pause a reader after this many reads; 0 disables corking")
	flag.DurationVar(&cfg.corkDuration, "cork", 250*time.Millisecond, "reader cork pause duration")
	flag.DurationVar(&cfg.writeInterval, "write-interval", 200*time.Millisecond, "delay between writes per stream; 0 writes as fast as possible")
	flag.DurationVar(&cfg.disconnectWait, "disconnect-wait", 5*time.Second, "time to wait for server-side cleanup after device disconnect")
	flag.DurationVar(&cfg.reportInterval, "report", 5*time.Second, "progress report interval")
	flag.DurationVar(&cfg.readTimeout, "read-timeout", 10*time.Second, "per-read timeout while the run is active")
	flag.DurationVar(&cfg.forceGCInterval, "force-gc-interval", 0, "force runtime.GC on this interval and emit a post-GC sample; 0 disables")
	flag.DurationVar(&cfg.freeOSMemInterval, "free-os-memory-interval", 0, "force debug.FreeOSMemory on this interval and emit a post-free sample; 0 disables")
	flag.Int64Var(&cfg.maxHeapGrowthMB, "max-heap-growth-mb", 128, "fail if post-GC heap allocation grows beyond this many MB")
	flag.Int64Var(&cfg.maxHeapSysGrowthMB, "max-heap-sys-growth-mb", 0, "fail if post-GC HeapSys grows beyond this many MB; 0 disables")
	flag.Uint64Var(&cfg.serviceID, "service", defaultServiceID, "giznet service id to use")
	flag.StringVar(&cipherMode, "cipher", "", "giznet cipher mode: empty, chacha_poly, aes_256_gcm, plaintext")
	flag.BoolVar(&cfg.requireOffline, "require-offline", false, "also require the low-level UDP peer state to become offline or removed")
	flag.StringVar(&cfg.pprofAddr, "pprof-addr", "", "optional pprof listen address, for example 127.0.0.1:6060 or 127.0.0.1:0")
	flag.StringVar(&cfg.memCSV, "mem-csv", "", "optional path for periodic memory samples as CSV")
	flag.Parse()

	cfg.cipherMode = giznet.CipherMode(cipherMode)
	return cfg
}

func run(cfg config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	stopPprof, err := startPprof(cfg.pprofAddr)
	if err != nil {
		return err
	}
	defer stopPprof()

	switch cfg.mode {
	case "trickle":
		return runTrickle(cfg)
	case "disconnect":
		return runDisconnect(cfg)
	case "all":
		if err := runTrickle(cfg); err != nil {
			return err
		}
		return runDisconnect(cfg)
	default:
		return fmt.Errorf("unknown -mode %q; want trickle, disconnect, or all", cfg.mode)
	}
}

func runTrickle(cfg config) error {
	fmt.Printf("corktest trickle start duration=%s streams=%d chunk=%d write_interval=%s cork_every=%d cork=%s service=%d cipher=%q\n",
		cfg.duration, cfg.streams, cfg.chunkSize, cfg.writeInterval, cfg.corkEvery, cfg.corkDuration, cfg.serviceID, cfg.cipherMode)

	pair, err := newEndpointPair(cfg)
	if err != nil {
		return err
	}
	defer pair.Close()

	pairs, err := openStreamPairs(pair.serverConn, pair.clientConn, cfg)
	if err != nil {
		return err
	}
	for _, pair := range pairs {
		defer pair.client.Close()
		defer pair.server.Close()
	}

	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	baseMem := readMem()
	baseGoroutines := runtime.NumGoroutine()
	memLog, err := openMemLog(cfg.memCSV)
	if err != nil {
		return err
	}
	defer memLog.Close()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.duration)
	defer cancel()

	var stats counters
	errCh := make(chan error, cfg.streams*4)
	var wg sync.WaitGroup
	for _, pair := range pairs {
		pair := pair
		wg.Add(4)
		go writePattern(ctx, &wg, errCh, pair.client, pair.id, 0x11, cfg)
		go readPattern(ctx, &wg, errCh, pair.server, pair.id, 0x11, cfg, stats.clientToServerBytes.Add, stats.clientToServerReads.Add)
		go writePattern(ctx, &wg, errCh, pair.server, pair.id, 0x77, cfg)
		go readPattern(ctx, &wg, errCh, pair.client, pair.id, 0x77, cfg, stats.serverToClientBytes.Add, stats.serverToClientReads.Add)
	}

	reportDone := make(chan struct{})
	start := time.Now()
	memLog.Write(start, start, &stats, baseMem, "base")
	go reportLoop(reportDone, cfg.reportInterval, start, &stats, memLog)
	go memControlLoop(reportDone, cfg, start, &stats, memLog)

	<-ctx.Done()
	for _, pair := range pairs {
		_ = pair.client.Close()
		_ = pair.server.Close()
	}
	wg.Wait()
	close(reportDone)
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	runtime.GC()
	time.Sleep(250 * time.Millisecond)
	endMem := readMem()
	totalBytes := stats.clientToServerBytes.Load() + stats.serverToClientBytes.Load()
	totalReads := stats.clientToServerReads.Load() + stats.serverToClientReads.Load()
	elapsed := cfg.duration.Seconds()
	heapGrowth := int64(endMem.heapAlloc) - int64(baseMem.heapAlloc)
	heapSysGrowth := int64(endMem.heapSys) - int64(baseMem.heapSys)
	heapInuseGrowth := int64(endMem.heapInuse) - int64(baseMem.heapInuse)
	goroutineGrowth := runtime.NumGoroutine() - baseGoroutines

	fmt.Printf("corktest trickle done bytes=%d reads=%d throughput=%.2fMiB/s c2s=%d s2c=%d\n",
		totalBytes, totalReads, float64(totalBytes)/(1024*1024)/elapsed, stats.clientToServerBytes.Load(), stats.serverToClientBytes.Load())
	fmt.Printf("memory base_heap_alloc=%d end_heap_alloc=%d heap_alloc_growth=%d base_heap_inuse=%d end_heap_inuse=%d heap_inuse_growth=%d base_heap_sys=%d end_heap_sys=%d heap_sys_growth=%d base_heap_idle=%d end_heap_idle=%d base_heap_released=%d end_heap_released=%d base_objects=%d end_objects=%d base_num_gc=%d end_num_gc=%d goroutine_growth=%d\n",
		baseMem.heapAlloc, endMem.heapAlloc, heapGrowth,
		baseMem.heapInuse, endMem.heapInuse, heapInuseGrowth,
		baseMem.heapSys, endMem.heapSys, heapSysGrowth,
		baseMem.heapIdle, endMem.heapIdle,
		baseMem.heapReleased, endMem.heapReleased,
		baseMem.objects, endMem.objects,
		baseMem.numGC, endMem.numGC,
		goroutineGrowth)

	if heapGrowth > cfg.maxHeapGrowthMB*1024*1024 {
		return fmt.Errorf("heap growth %d bytes exceeded limit %d MiB", heapGrowth, cfg.maxHeapGrowthMB)
	}
	if cfg.maxHeapSysGrowthMB > 0 && heapSysGrowth > cfg.maxHeapSysGrowthMB*1024*1024 {
		return fmt.Errorf("HeapSys growth %d bytes exceeded limit %d MiB", heapSysGrowth, cfg.maxHeapSysGrowthMB)
	}
	if goroutineGrowth > 64 {
		return fmt.Errorf("goroutine growth %d exceeded limit 64", goroutineGrowth)
	}
	return nil
}

func runDisconnect(cfg config) error {
	fmt.Printf("corktest disconnect start streams=%d service=%d wait=%s cipher=%q\n",
		cfg.streams, cfg.serviceID, cfg.disconnectWait, cfg.cipherMode)

	pair, err := newEndpointPair(cfg)
	if err != nil {
		return err
	}
	defer pair.server.Close()

	streams, err := openStreamPairs(pair.serverConn, pair.clientConn, cfg)
	if err != nil {
		pair.Close()
		return err
	}

	serverReadErrCh := make(chan error, len(streams))
	for _, stream := range streams {
		serverStream := stream.server
		go func() {
			buf := make([]byte, 1)
			_, err := serverStream.Read(buf)
			serverReadErrCh <- err
		}()
	}

	baseMem := readMem()
	baseGoroutines := runtime.NumGoroutine()
	if err := pair.client.Close(); err != nil && !isClosedErr(err) {
		return fmt.Errorf("close client listener: %w", err)
	}

	deadline := time.Now().Add(cfg.disconnectWait)
	var lastPeerInfo *giznet.PeerInfo
	var peerHandlePresent bool
	readErrors := make([]error, 0, len(streams))
	for time.Now().Before(deadline) {
		for {
			select {
			case err := <-serverReadErrCh:
				readErrors = append(readErrors, err)
			default:
				goto drained
			}
		}
	drained:
		lastPeerInfo = pair.server.UDP().PeerInfo(pair.clientKey.Public)
		_, peerHandlePresent = pair.server.Peer(pair.clientKey.Public)
		if len(readErrors) == len(streams) &&
			!peerHandlePresent &&
			(lastPeerInfo == nil || lastPeerInfo.State == giznet.PeerStateOffline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	naturalReadErrors := len(readErrors)
	for _, stream := range streams {
		_ = stream.server.Close()
		_ = stream.client.Close()
	}
	_ = pair.serverConn.Close()

	for len(readErrors) < len(streams) {
		select {
		case err := <-serverReadErrCh:
			readErrors = append(readErrors, err)
		default:
			if len(readErrors) < len(streams) {
				return fmt.Errorf("server stream reads unblocked=%d/%d after client disconnect", len(readErrors), len(streams))
			}
		}
	}
	if naturalReadErrors < len(streams) {
		return fmt.Errorf("server stream reads unblocked=%d/%d after client disconnect", naturalReadErrors, len(streams))
	}
	for i, err := range readErrors {
		if err == nil {
			return fmt.Errorf("server stream read[%d] returned nil after client disconnect", i)
		}
	}

	runtime.GC()
	time.Sleep(250 * time.Millisecond)
	endMem := readMem()
	heapGrowth := int64(endMem.heapAlloc) - int64(baseMem.heapAlloc)
	goroutineGrowth := runtime.NumGoroutine() - baseGoroutines

	_, peerHandlePresent = pair.server.Peer(pair.clientKey.Public)
	lastPeerInfo = pair.server.UDP().PeerInfo(pair.clientKey.Public)
	fmt.Printf("disconnect observed peer_handle_present=%t peer_info=%s heap_growth=%d goroutine_growth=%d\n",
		peerHandlePresent, formatPeerInfo(lastPeerInfo), heapGrowth, goroutineGrowth)

	if peerHandlePresent {
		return errors.New("server listener still owns peer Conn after device disconnect")
	}
	if cfg.requireOffline && lastPeerInfo != nil && lastPeerInfo.State != giznet.PeerStateOffline {
		return fmt.Errorf("server peer state after disconnect = %s, want offline or removed", lastPeerInfo.State)
	}
	if heapGrowth > cfg.maxHeapGrowthMB*1024*1024 {
		return fmt.Errorf("heap growth %d bytes exceeded limit %d MiB", heapGrowth, cfg.maxHeapGrowthMB)
	}
	if goroutineGrowth > 64 {
		return fmt.Errorf("goroutine growth %d exceeded limit 64", goroutineGrowth)
	}
	fmt.Println("corktest disconnect done")
	return nil
}

func (cfg config) validate() error {
	if cfg.duration <= 0 {
		return errors.New("-duration must be positive")
	}
	if cfg.streams <= 0 {
		return errors.New("-streams must be positive")
	}
	if cfg.chunkSize <= 0 {
		return errors.New("-chunk must be positive")
	}
	if cfg.corkEvery < 0 {
		return errors.New("-cork-every cannot be negative")
	}
	if cfg.corkDuration < 0 {
		return errors.New("-cork cannot be negative")
	}
	if cfg.writeInterval < 0 {
		return errors.New("-write-interval cannot be negative")
	}
	if cfg.disconnectWait <= 0 {
		return errors.New("-disconnect-wait must be positive")
	}
	if cfg.reportInterval <= 0 {
		return errors.New("-report must be positive")
	}
	if cfg.readTimeout <= 0 {
		return errors.New("-read-timeout must be positive")
	}
	if cfg.forceGCInterval < 0 {
		return errors.New("-force-gc-interval cannot be negative")
	}
	if cfg.freeOSMemInterval < 0 {
		return errors.New("-free-os-memory-interval cannot be negative")
	}
	if cfg.maxHeapGrowthMB <= 0 {
		return errors.New("-max-heap-growth-mb must be positive")
	}
	if cfg.maxHeapSysGrowthMB < 0 {
		return errors.New("-max-heap-sys-growth-mb cannot be negative")
	}
	return nil
}

func listen(key *giznet.KeyPair, cipherMode giznet.CipherMode) (*giznet.Listener, error) {
	return (&giznet.ListenConfig{
		Addr:           "127.0.0.1:0",
		SecurityPolicy: allowAllPolicy{},
		CipherMode:     cipherMode,
	}).Listen(key)
}

func newEndpointPair(cfg config) (*endpointPair, error) {
	serverKey, err := giznet.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate server key: %w", err)
	}
	clientKey, err := giznet.GenerateKeyPair()
	if err != nil {
		return nil, fmt.Errorf("generate client key: %w", err)
	}

	server, err := listen(serverKey, cfg.cipherMode)
	if err != nil {
		return nil, fmt.Errorf("listen server: %w", err)
	}
	client, err := listen(clientKey, cfg.cipherMode)
	if err != nil {
		_ = server.Close()
		return nil, fmt.Errorf("listen client: %w", err)
	}

	serverConnCh := make(chan *giznet.Conn, 1)
	acceptErrCh := make(chan error, 1)
	go func() {
		conn, err := server.Accept()
		if err != nil {
			acceptErrCh <- err
			return
		}
		serverConnCh <- conn
	}()

	clientConn, err := client.Dial(serverKey.Public, server.HostInfo().Addr)
	if err != nil {
		_ = client.Close()
		_ = server.Close()
		return nil, fmt.Errorf("dial server: %w", err)
	}

	var serverConn *giznet.Conn
	select {
	case serverConn = <-serverConnCh:
	case err := <-acceptErrCh:
		_ = clientConn.Close()
		_ = client.Close()
		_ = server.Close()
		return nil, fmt.Errorf("accept server conn: %w", err)
	case <-time.After(10 * time.Second):
		_ = clientConn.Close()
		_ = client.Close()
		_ = server.Close()
		return nil, errors.New("accept server conn: timeout")
	}

	return &endpointPair{
		serverKey:  serverKey,
		clientKey:  clientKey,
		server:     server,
		client:     client,
		serverConn: serverConn,
		clientConn: clientConn,
	}, nil
}

func (p *endpointPair) Close() {
	if p == nil {
		return
	}
	if p.clientConn != nil {
		_ = p.clientConn.Close()
	}
	if p.serverConn != nil {
		_ = p.serverConn.Close()
	}
	if p.client != nil {
		_ = p.client.Close()
	}
	if p.server != nil {
		_ = p.server.Close()
	}
}

func openStreamPairs(serverConn, clientConn *giznet.Conn, cfg config) ([]streamPair, error) {
	listener := serverConn.ListenService(cfg.serviceID)
	defer listener.Close()

	accepted := make(chan net.Conn, cfg.streams)
	errCh := make(chan error, 1)
	go func() {
		for range cfg.streams {
			stream, err := listener.Accept()
			if err != nil {
				errCh <- err
				return
			}
			accepted <- stream
		}
	}()

	pairs := make([]streamPair, 0, cfg.streams)
	for i := range cfg.streams {
		clientStream, err := clientConn.Dial(cfg.serviceID)
		if err != nil {
			return nil, fmt.Errorf("open client stream %d: %w", i, err)
		}
		var serverStream net.Conn
		select {
		case serverStream = <-accepted:
		case err := <-errCh:
			_ = clientStream.Close()
			return nil, fmt.Errorf("accept stream %d: %w", i, err)
		case <-time.After(10 * time.Second):
			_ = clientStream.Close()
			return nil, fmt.Errorf("accept stream %d: timeout", i)
		}
		pairs = append(pairs, streamPair{id: i, client: clientStream, server: serverStream})
	}
	return pairs, nil
}

func writePattern(ctx context.Context, wg *sync.WaitGroup, errCh chan<- error, conn net.Conn, streamID int, seed byte, cfg config) {
	defer wg.Done()

	buf := make([]byte, cfg.chunkSize)
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		fillPattern(buf, streamID, seed, offset)
		n, err := conn.Write(buf)
		if err != nil {
			if ctx.Err() != nil || isClosedErr(err) {
				return
			}
			errCh <- fmt.Errorf("stream=%d seed=%02x write offset=%d: %w", streamID, seed, offset, err)
			return
		}
		if n != len(buf) {
			errCh <- fmt.Errorf("stream=%d seed=%02x short write n=%d want=%d", streamID, seed, n, len(buf))
			return
		}
		offset += int64(n)
		if cfg.writeInterval > 0 {
			timer := time.NewTimer(cfg.writeInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func readPattern(
	ctx context.Context,
	wg *sync.WaitGroup,
	errCh chan<- error,
	conn net.Conn,
	streamID int,
	seed byte,
	cfg config,
	addBytes func(int64) int64,
	addReads func(int64) int64,
) {
	defer wg.Done()

	buf := make([]byte, cfg.chunkSize*2)
	var offset int64
	var reads int64
	for {
		if err := conn.SetReadDeadline(time.Now().Add(cfg.readTimeout)); err != nil && ctx.Err() == nil {
			errCh <- fmt.Errorf("stream=%d seed=%02x set read deadline: %w", streamID, seed, err)
			return
		}
		n, err := conn.Read(buf)
		if n > 0 {
			if patternErr := checkPattern(buf[:n], streamID, seed, offset); patternErr != nil {
				errCh <- patternErr
				return
			}
			offset += int64(n)
			reads++
			addBytes(int64(n))
			addReads(1)
			if cfg.corkEvery > 0 && reads%int64(cfg.corkEvery) == 0 {
				timer := time.NewTimer(cfg.corkDuration)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}
		if err != nil {
			if ctx.Err() != nil || isClosedErr(err) {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				errCh <- fmt.Errorf("stream=%d seed=%02x read timeout after %s at offset=%d", streamID, seed, cfg.readTimeout, offset)
				return
			}
			if errors.Is(err, io.EOF) {
				return
			}
			errCh <- fmt.Errorf("stream=%d seed=%02x read offset=%d: %w", streamID, seed, offset, err)
			return
		}
	}
}

func reportLoop(done <-chan struct{}, interval time.Duration, start time.Time, stats *counters, memLog *memLog) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastBytes int64
	var lastTime = start
	for {
		select {
		case <-done:
			return
		case now := <-ticker.C:
			total := stats.clientToServerBytes.Load() + stats.serverToClientBytes.Load()
			deltaBytes := total - lastBytes
			deltaSeconds := now.Sub(lastTime).Seconds()
			mem := readMem()
			fmt.Printf("progress elapsed=%s total=%d interval=%.2fMiB/s c2s=%d s2c=%d heap_alloc=%d heap_inuse=%d heap_idle=%d heap_released=%d heap_sys=%d objects=%d num_gc=%d goroutines=%d\n",
				now.Sub(start).Round(time.Millisecond),
				total,
				float64(deltaBytes)/(1024*1024)/deltaSeconds,
				stats.clientToServerBytes.Load(),
				stats.serverToClientBytes.Load(),
				mem.heapAlloc,
				mem.heapInuse,
				mem.heapIdle,
				mem.heapReleased,
				mem.heapSys,
				mem.objects,
				mem.numGC,
				mem.goroutines,
			)
			memLog.Write(start, now, stats, mem, "report")
			lastBytes = total
			lastTime = now
		}
	}
}

func memControlLoop(done <-chan struct{}, cfg config, start time.Time, stats *counters, memLog *memLog) {
	var forceGCTicker, freeOSMemTicker *time.Ticker
	var forceGC, freeOSMem <-chan time.Time
	if cfg.forceGCInterval > 0 {
		forceGCTicker = time.NewTicker(cfg.forceGCInterval)
		forceGC = forceGCTicker.C
		defer forceGCTicker.Stop()
	}
	if cfg.freeOSMemInterval > 0 {
		freeOSMemTicker = time.NewTicker(cfg.freeOSMemInterval)
		freeOSMem = freeOSMemTicker.C
		defer freeOSMemTicker.Stop()
	}
	if forceGC == nil && freeOSMem == nil {
		return
	}

	for {
		select {
		case <-done:
			return
		case now := <-forceGC:
			runtime.GC()
			time.Sleep(100 * time.Millisecond)
			mem := readMem()
			printMemControlSample("force_gc", start, now, stats, mem)
			memLog.Write(start, now, stats, mem, "force_gc")
		case now := <-freeOSMem:
			debug.FreeOSMemory()
			time.Sleep(100 * time.Millisecond)
			mem := readMem()
			printMemControlSample("free_os_memory", start, now, stats, mem)
			memLog.Write(start, now, stats, mem, "free_os_memory")
		}
	}
}

func printMemControlSample(event string, start time.Time, now time.Time, stats *counters, mem memSnapshot) {
	total := stats.clientToServerBytes.Load() + stats.serverToClientBytes.Load()
	fmt.Printf("%s elapsed=%s total=%d c2s=%d s2c=%d heap_alloc=%d heap_inuse=%d heap_idle=%d heap_released=%d heap_sys=%d objects=%d num_gc=%d goroutines=%d\n",
		event,
		now.Sub(start).Round(time.Millisecond),
		total,
		stats.clientToServerBytes.Load(),
		stats.serverToClientBytes.Load(),
		mem.heapAlloc,
		mem.heapInuse,
		mem.heapIdle,
		mem.heapReleased,
		mem.heapSys,
		mem.objects,
		mem.numGC,
		mem.goroutines,
	)
}

func fillPattern(dst []byte, streamID int, seed byte, offset int64) {
	for i := range dst {
		dst[i] = patternByte(streamID, seed, offset+int64(i))
	}
}

func checkPattern(data []byte, streamID int, seed byte, offset int64) error {
	for i, got := range data {
		want := patternByte(streamID, seed, offset+int64(i))
		if got != want {
			return fmt.Errorf("stream=%d seed=%02x pattern mismatch offset=%d got=%02x want=%02x", streamID, seed, offset+int64(i), got, want)
		}
	}
	return nil
}

func patternByte(streamID int, seed byte, offset int64) byte {
	return byte(int(seed) + streamID*17 + int(offset%251))
}

func isClosedErr(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, giznet.ErrClosed) ||
		errors.Is(err, giznet.ErrConnClosed)
}

func readMem() memSnapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return memSnapshot{
		heapAlloc:    m.HeapAlloc,
		heapInuse:    m.HeapInuse,
		heapIdle:     m.HeapIdle,
		heapReleased: m.HeapReleased,
		heapSys:      m.HeapSys,
		objects:      m.HeapObjects,
		totalAlloc:   m.TotalAlloc,
		mallocs:      m.Mallocs,
		frees:        m.Frees,
		numGC:        m.NumGC,
		goroutines:   runtime.NumGoroutine(),
	}
}

func formatPeerInfo(info *giznet.PeerInfo) string {
	if info == nil {
		return "<nil>"
	}
	return fmt.Sprintf("{state:%s rx:%d tx:%d last_seen:%s}", info.State, info.RxBytes, info.TxBytes, info.LastSeen.Format(time.RFC3339Nano))
}

func startPprof(addr string) (func(), error) {
	if addr == "" {
		return func() {}, nil
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen pprof: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{Handler: mux}
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "pprof server error: %v\n", err)
		}
	}()
	fmt.Printf("pprof listening on http://%s/debug/pprof/\n", listener.Addr())
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}, nil
}

type memLog struct {
	file *os.File
	mu   sync.Mutex
}

func openMemLog(path string) (*memLog, error) {
	if path == "" {
		return &memLog{}, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create mem csv: %w", err)
	}
	log := &memLog{file: file}
	fmt.Fprintln(file, "elapsed_ms,total_bytes,c2s_bytes,s2c_bytes,heap_alloc,heap_inuse,heap_idle,heap_released,heap_sys,heap_objects,total_alloc,mallocs,frees,num_gc,goroutines,event")
	return log, nil
}

func (l *memLog) Write(start, now time.Time, stats *counters, mem memSnapshot, event string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	total := stats.clientToServerBytes.Load() + stats.serverToClientBytes.Load()
	fmt.Fprintf(l.file, "%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%d,%s\n",
		now.Sub(start).Milliseconds(),
		total,
		stats.clientToServerBytes.Load(),
		stats.serverToClientBytes.Load(),
		mem.heapAlloc,
		mem.heapInuse,
		mem.heapIdle,
		mem.heapReleased,
		mem.heapSys,
		mem.objects,
		mem.totalAlloc,
		mem.mallocs,
		mem.frees,
		mem.numGC,
		mem.goroutines,
		event,
	)
}

func (l *memLog) Close() {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.file.Close()
	l.file = nil
}
