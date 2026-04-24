package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	pb "auto-monitor/proto"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// App struct
type App struct {
	ctx           context.Context
	conn          *grpc.ClientConn
	client        pb.MonitorServiceClient
	codeApp       string
	isMonitoring  bool
	monitoringMu  sync.Mutex
	statusMessage string
	lastMetrics   *MetricsData
}

type AuthResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type MetricsData struct {
	CPUUsage  float64 `json:"cpu_usage"`
	RAMUsage  float64 `json:"ram_usage"`
	DiskUsage float64 `json:"disk_usage"`
	NetworkTX float64 `json:"network_tx"`
	NetworkRX float64 `json:"network_rx"`
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		statusMessage: "Aguardando login...",
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	
	// Tentar conectar ao servidor gRPC (localhost:50051 por padrão)
	target := "31.97.175.114:50051"
	fmt.Printf("[DEBUG] Conectando ao servidor gRPC em: %s\n", target)
	conn, err := grpc.Dial(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		a.statusMessage = "Erro ao conectar ao servidor"
		return
	}
	a.conn = conn
	a.client = pb.NewMonitorServiceClient(conn)
}

// Authenticate checks the license code and starts monitoring if valid
func (a *App) Authenticate(code string) AuthResponse {
	if a.client == nil {
		return AuthResponse{Success: false, Message: "Servidor gRPC não encontrado"}
	}

	code = strings.TrimSpace(code)
	if code == "" {
		return AuthResponse{Success: false, Message: "Código não pode ser vazio"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := a.client.Authenticate(ctx, &pb.AuthRequest{CodeApp: code})
	if err != nil {
		fmt.Printf("[ERROR] Falha na autenticação gRPC: %v\n", err)
		return AuthResponse{Success: false, Message: "Erro de comunicação: " + err.Error()}
	}

	if !resp.Success {
		return AuthResponse{Success: false, Message: resp.Message}
	}

	a.codeApp = code
	a.statusMessage = "Conectado com sucesso"
	
	// Iniciar monitoramento em background
	go a.startMonitoringLoop()

	return AuthResponse{Success: true, Message: "Sucesso"}
}

func (a *App) startMonitoringLoop() {
	a.monitoringMu.Lock()
	if a.isMonitoring {
		a.monitoringMu.Unlock()
		return
	}
	a.isMonitoring = true
	a.monitoringMu.Unlock()

	defer func() {
		a.monitoringMu.Lock()
		a.isMonitoring = false
		a.monitoringMu.Unlock()
	}()

	stream, err := a.client.TransmitMetrics(context.Background())
	if err != nil {
		log.Printf("Erro ao iniciar stream: %v", err)
		return
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastNetIO *net.IOCountersStat

	for {
		select {
		case <-a.ctx.Done():
			a.client.Disconnect(context.Background(), &pb.DisconnectRequest{CodeApp: a.codeApp})
			return
		case <-ticker.C:
			// CPU
			percentages, _ := cpu.Percent(0, false)
			cpuUsage := percentages[0]

			// RAM
			vm, _ := mem.VirtualMemory()
			ramUsage := vm.UsedPercent

			// Disk
			d, _ := disk.Usage("/")
			diskUsage := d.UsedPercent

			// Network
			netIO, _ := net.IOCounters(false)
			var tx, rx float64
			if len(netIO) > 0 {
				if lastNetIO != nil {
					tx = float64(netIO[0].BytesSent - lastNetIO.BytesSent)
					rx = float64(netIO[0].BytesRecv - lastNetIO.BytesRecv)
				}
				lastNetIO = &netIO[0]
			}

			a.lastMetrics = &MetricsData{
				CPUUsage:  cpuUsage,
				RAMUsage:  ramUsage,
				DiskUsage: diskUsage,
				NetworkTX: tx,
				NetworkRX: rx,
			}

			err := stream.Send(&pb.MetricsRequest{
				CodeApp:   a.codeApp,
				CpuUsage:  cpuUsage,
				RamUsage:  ramUsage,
				DiskUsage: diskUsage,
				NetworkTx: tx,
				NetworkRx: rx,
			})

			if err != nil {
				log.Printf("Stream error: %v", err)
				a.statusMessage = "Conexão perdida, tentando reconectar..."
				return // Encerra loop para tentar novamente depois (ou o usuário reloga)
			}
		}
	}
}

// GetMetrics returns the last collected metrics to the frontend
func (a *App) GetMetrics() *MetricsData {
	return a.lastMetrics
}

// GetStatus returns the current status message
func (a *App) GetStatus() string {
	return a.statusMessage
}

// Quit handles cleanup before closing
func (a *App) Quit() {
	if a.conn != nil {
		if a.codeApp != "" {
			a.client.Disconnect(context.Background(), &pb.DisconnectRequest{CodeApp: a.codeApp})
		}
		a.conn.Close()
	}
	os.Exit(0)
}
