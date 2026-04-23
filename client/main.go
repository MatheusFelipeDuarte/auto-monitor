package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	pb "auto-monitor/proto"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\033[2J\033[H") // Limpa a tela e move cursor para o topo
		fmt.Println("🚀 AUTO MONITOR CLIENT")
		fmt.Println("═══════════════════════")

		conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			fmt.Printf("\n❌ Erro ao tentar conectar no endereço do servidor: %v\n", err)
			time.Sleep(3 * time.Second)
			continue
		}
		client := pb.NewMonitorServiceClient(conn)

		var codeApp string
		authenticated := false

		for !authenticated {
			fmt.Print("\n🔹 Digite o código da licença (code_app): ")
			input, _ := reader.ReadString('\n')
			codeApp = strings.TrimSpace(input)

			if codeApp == "" {
				fmt.Println("⚠️  O código não pode ser vazio.")
				continue
			}

			// Authenticate
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			resp, err := client.Authenticate(ctx, &pb.AuthRequest{CodeApp: codeApp})
			cancel()

			if err != nil {
				fmt.Printf("\n📡 Servidor Offline ou Inacessível. Tentando reconectar...\n")
				time.Sleep(2 * time.Second)
				break // Volta para o início do loop global para tentar nova conexão gRPC
			}

			if !resp.Success {
				fmt.Printf("\n❌ Falha na autenticação: %s\n", resp.Message)
				continue
			}

			authenticated = true
		}

		if !authenticated {
			conn.Close()
			continue
		}

		fmt.Println("\n✅ Conectado com sucesso! Iniciando transmissão...")

		// Handle Graceful Disconnect
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigChan
			fmt.Println("\n\n👋 Desconectando...")
			client.Disconnect(context.Background(), &pb.DisconnectRequest{CodeApp: codeApp})
			os.Exit(0)
		}()

		// Start streaming metrics
		stream, err := client.TransmitMetrics(context.Background())
		if err != nil {
			fmt.Printf("❌ Erro ao iniciar stream: %v\n", err)
			conn.Close()
			time.Sleep(2 * time.Second)
			continue
		}

		ticker := time.NewTicker(1 * time.Second)
		var lastNetIO *net.IOCountersStat
		streamError := false

		for range ticker.C {
			if streamError {
				break
			}

			// Coleta de métricas (CPU, RAM, etc)
			percentages, _ := cpu.Percent(0, false)
			cpuUsage := percentages[0]
			vm, _ := mem.VirtualMemory()
			ramUsage := vm.UsedPercent
			d, _ := disk.Usage("/")
			diskUsage := d.UsedPercent
			netIO, _ := net.IOCounters(false)
			var tx, rx float64
			if len(netIO) > 0 {
				if lastNetIO != nil {
					tx = float64(netIO[0].BytesSent - lastNetIO.BytesSent)
					rx = float64(netIO[0].BytesRecv - lastNetIO.BytesRecv)
				}
				lastNetIO = &netIO[0]
			}

			fmt.Printf("\r📊 Monitorando [%s] >> CPU: %.1f%% | RAM: %.1f%% | TX: %.1f KB/s ",
				codeApp, cpuUsage, ramUsage, tx/1024)

			err := stream.Send(&pb.MetricsRequest{
				CodeApp:   codeApp,
				CpuUsage:  cpuUsage,
				RamUsage:  ramUsage,
				DiskUsage: diskUsage,
				NetworkTx: tx,
				NetworkRx: rx,
			})

			if err != nil {
				fmt.Printf("\n\n🚨 Conexão perdida com o servidor. Motivo: %v\n", err)
				fmt.Println("Retornando para a tela de autenticação em 3 segundos...")
				streamError = true
				time.Sleep(3 * time.Second)
			}
		}

		ticker.Stop()
		conn.Close()
	}
}
