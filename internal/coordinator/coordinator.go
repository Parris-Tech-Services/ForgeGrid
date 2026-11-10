package coordinator

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"forgegrid/internal/network"
	"forgegrid/internal/store"
	"forgegrid/internal/ui"
)

type Coordinator struct {
	Store       *store.Store
	IP          string
	Insecure    bool
	Fingerprint string
	Listener    net.Listener
	stopCh      chan struct{}
	stopOnce    sync.Once
	statusWG    sync.WaitGroup
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func New(s *store.Store, insecure bool) *Coordinator {
	return &Coordinator{
		Store:    s,
		IP:       getOutboundIP(),
		Insecure: insecure,
		stopCh: make(chan struct{}),
	}
}

func (c *Coordinator) Start(port string) error {
	mux := http.NewServeMux()

	// Ensure identity and TLS cert
	c.Store.Mu.Lock()
	if c.Store.CoordinatorCfg.Identity == "" {
		c.Store.CoordinatorCfg.Identity = fmt.Sprintf("ForgeGrid-%d", time.Now().UnixNano()) // fallback if rand fails later
		c.Store.Save()
	}

	if len(c.Store.CoordinatorCfg.CertPEM) == 0 {
		certPEM, keyPEM, fp, err := network.GenerateSelfSignedCert()
		if err != nil {
			c.Store.Mu.Unlock()
			return fmt.Errorf("failed to generate TLS cert: %w", err)
		}
		c.Store.CoordinatorCfg.CertPEM = certPEM
		c.Store.CoordinatorCfg.KeyPEM = keyPEM
		c.Fingerprint = fp
		c.Store.Save()
	} else {
		fp, err := network.FingerprintFromPEM(c.Store.CoordinatorCfg.CertPEM)
		if err != nil {
			c.Store.Mu.Unlock()
			return err
		}
		c.Fingerprint = fp
	}
	c.Store.Mu.Unlock()

	mux.HandleFunc("/api/coordinator/start", c.handleStart)
	mux.HandleFunc("/api/coordinator/status", c.handleStatus)
	mux.HandleFunc("/api/pairing/code", c.handleGenerateCode)
	mux.HandleFunc("/api/workers/pair", c.handlePair)
	mux.HandleFunc("/api/workers/heartbeat", c.handleHeartbeat)
	mux.HandleFunc("/api/workers", c.handleListWorkers)
	mux.HandleFunc("/api/jobs/test", c.handleTestJob)
	mux.HandleFunc("/api/jobs", c.handleListJobs)
	mux.HandleFunc("/api/jobs/", c.handleJobAction)
	mux.HandleFunc("/api/jobs/manifest", c.handleManifest)

	// Serve UI
	mux.Handle("/", http.FileServer(http.FS(ui.DashboardFS)))

	var addr string
	if c.Listener != nil {
		addr = c.Listener.Addr().String()
	} else {
		addr = fmt.Sprintf("0.0.0.0:%s", port)
	}

	c.statusWG.Add(1)
	go func() { defer c.statusWG.Done(); c.checkWorkerStatus() }()

	scheme := "https"
	if c.Insecure {
		scheme = "http"
	}
	uiURL := fmt.Sprintf("%s://%s:%s", scheme, c.IP, port)
	fmt.Printf("Coordinator starting on %s (LAN IP: %s)\n", addr, c.IP)
	if !c.Insecure {
		fmt.Printf("TLS Fingerprint: %s\n", c.Fingerprint)
	}

	ui.OpenBrowser(uiURL)

	if c.Listener != nil {
		if c.Insecure {
			return http.Serve(c.Listener, mux)
		}
		cert, err := tls.X509KeyPair(c.Store.CoordinatorCfg.CertPEM, c.Store.CoordinatorCfg.KeyPEM)
		if err != nil {
			return err
		}
		server := &http.Server{
			Handler: mux,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}
		return server.ServeTLS(c.Listener, "", "")
	}

	if c.Insecure {
		return http.ListenAndServe(addr, mux)
	}

	// Create tls cert from PEM
	cert, err := tls.X509KeyPair(c.Store.CoordinatorCfg.CertPEM, c.Store.CoordinatorCfg.KeyPEM)
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	return server.ListenAndServeTLS("", "")
}

// Stop stops the status monitor and closes an explicitly supplied listener.
// It does not grant network access or alter the coordinator's pairing model.
func (c *Coordinator) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
	if c.Listener != nil { _ = c.Listener.Close() }
	c.statusWG.Wait()
}

func (c *Coordinator) checkWorkerStatus() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.Store.Mu.Lock()
			changed := false
			for _, w := range c.Store.Workers {
				if w.Status == "online" && time.Since(w.LastSeen) > 15*time.Second {
					w.Status = "offline"
					changed = true
				}
			}
			if changed { c.Store.Save() }
			c.Store.Mu.Unlock()
		}
	}
}
