package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/LINBIT/drbdtop/pkg/collect"
	"github.com/LINBIT/drbdtop/pkg/resource"
	"github.com/LINBIT/drbdtop/pkg/update"
)

const maxBufferSize = 1500

var message = []byte("Sometimes even to live is an act of courage.")

func communicateUDP(listen_addr string, send_addr string, side int) (err error) {
	toUdp, err := net.ResolveUDPAddr("udp", send_addr)
	udpSock, err := net.ListenPacket("udp", listen_addr)
	if err != nil {
		return
	}
	defer udpSock.Close()

	buffer := make([]byte, maxBufferSize)
	if side == 1 {
		go func() {
			for {
				n, _, _ := udpSock.ReadFrom(buffer)
				if bytes.Compare(buffer[:n], message) == 0 {
					fmt.Printf("Message received: %s\n", buffer[:n])
					args := []string{"/usr/bin/systemctl", "isolate", "--no-block", "cluster-active.target"}
					execErr := syscall.Exec("/usr/bin/systemctl", args, os.Environ())
					if execErr != nil {
						panic(execErr)
					}
					os.Exit(0)
				}
			}
		}()
	}
	for {
		if side != 1 {
			udpSock.WriteTo(message, toUdp)
		}
		time.Sleep(1 * time.Second)
	}
}

func checkDrbd() {
	errors := make(chan error, 100)
	events := make(chan resource.Event, 5)
	duration := time.Second * 1

	input := collect.Events2Poll{Interval: duration}
	go input.Collect(events, errors)

	resources := update.NewResourceCollection(duration)
	go func() {
		for {
			<-time.After(duration)
			resources.UpdateList()
			resources.RLock()
			for _, r := range resources.List {
				r.RLock()
				for _, c := range r.Connections {
					if c.Role == "Primary" {
						args := []string{"/usr/bin/systemctl", "isolate", "--no-block", "multi-user.target"}
						env := os.Environ()
						execErr := syscall.Exec("/usr/bin/systemctl", args, env)
						if execErr != nil {
							panic(execErr)
						}
						os.Exit(1)
					}
				}
				r.RUnlock()
			}
			resources.RUnlock()
		}
	}()
	for m := range events {
		resources.Update(m)
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {

	port, _ := strconv.Atoi(getEnv("PORT", "10000"))
	portstr := getEnv("PORT", "10000")
	side, _ := strconv.Atoi(os.Getenv("KS_side"))
	server1_sync_ip := getEnv("SERVER1_SYNC_IP", "10.10.10.1")
	server2_sync_ip := getEnv("SERVER2_SYNC_IP", "10.10.10.2")
	fmt.Printf("Running on server%d ...\n", side)

	var listen_ip string
	var send_ip string
	if side == 1 {
		listen_ip = server1_sync_ip
		send_ip = server2_sync_ip
	} else {
		listen_ip = server2_sync_ip
		send_ip = server1_sync_ip
	}
	fmt.Printf("Listening IP address is %s on port %d ...", listen_ip, port)
	fmt.Printf("Sending to IP address %s ...", send_ip)

	go checkDrbd()
	time.Sleep(5 * time.Second)
	communicateUDP(listen_ip+":"+portstr, send_ip+":"+portstr, side)

	os.Exit(0)
}
