package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
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
	fmt.Print("Digite o código da licença (code_app): ")
	codeApp, _ := reader.ReadString('\n')
	codeApp = strings.TrimSpace(codeApp)

	if codeApp == "" {
		log.Fatal("Código da licença não pode ser vazio")
	}

	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewMonitorServiceClient(conn)

	// Authenticate
	resp, err := client.Authenticate(context.Background(), &pb.AuthRequest{CodeApp: codeApp})
	if err != nil || !resp.Success {
		log.Fatalf("Falha na autenticação: %v", err)
	}
	fmt.Println("Conectado com sucesso!")

	// Handle Graceful Disconnect
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nDesconectando...")
		client.Disconnect(context.Background(), &pb.DisconnectRequest{CodeApp: codeApp})
		os.Exit(0)
	}()

	// Start streaming metrics
	stream, err := client.TransmitMetrics(context.Background())
	if err != nil {
		log.Fatalf("Error starting stream: %v", err)
	}

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	var lastNetIO *net.IOCountersStat

	for range ticker.C {
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

		fmt.Printf("\rCPU: %.2f%% | RAM: %.2f%% | Disk: %.2f%% | TX: %.2f KB/s | RX: %.2f KB/s", 
			cpuUsage, ramUsage, diskUsage, tx/1024, rx/1024)

		err := stream.Send(&pb.MetricsRequest{
			CodeApp:   codeApp,
			CpuUsage:  cpuUsage,
			RamUsage:  ramUsage,
			DiskUsage: diskUsage,
			NetworkTx: tx,
			NetworkRx: rx,
		})

		if err != nil {
			log.Printf("Stream send error: %v", err)
			return
		}
	}
}
