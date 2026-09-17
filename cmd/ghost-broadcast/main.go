package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

const baud = 115200

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ghost-broadcast:", err)
		os.Exit(1)
	}
}

func run() error {
	device := flag.String("device", "", "modem serial device")
	dataDir := flag.String("data-dir", "./data/broadcaster", "publication storage directory")
	flag.Parse()
	if *device == "" {
		return errors.New("--device is required")
	}
	link, err := publication.OpenLink(*device, baud)
	if err != nil {
		return err
	}
	defer link.Close()
	store := publication.NewStore(*dataDir)
	sessions := make(map[publication.ID]*publication.Assembly)
	fmt.Printf("broadcaster ready: device=%s data=%s\n", *device, *dataDir)
	for {
		packet, err := link.Receive(-1)
		if err != nil {
			return err
		}
		switch packet.Type {
		case publication.TypeManifest:
			manifest, err := publication.DecodeManifest(packet)
			if err != nil {
				sendError(link, packet.ID, "invalid manifest")
				continue
			}
			fmt.Printf("publication announced: %s file=%s bytes=%d chunks=%d\n", manifest.ID, manifest.Filename, manifest.Length, manifest.ChunkCount)
			if store.Complete(manifest.ID) {
				sendSimple(link, publication.TypeComplete, manifest.ID)
				continue
			}
			assembly, err := store.Begin(manifest)
			if err != nil {
				sendError(link, packet.ID, err.Error())
				continue
			}
			sessions[manifest.ID] = assembly
			if assembly.Complete() {
				commit(link, store, sessions, assembly, true)
				continue
			}
			sendReceipt(link, assembly)
		case publication.TypeChunk:
			assembly := sessions[packet.ID]
			if assembly == nil {
				sendError(link, packet.ID, "manifest required")
				continue
			}
			index, data, err := publication.DecodeChunk(packet)
			if err != nil || store.Add(assembly, index, data) != nil {
				sendError(link, packet.ID, "invalid chunk")
				continue
			}
			fmt.Printf("received chunk %d/%d\n", index+1, assembly.Manifest.ChunkCount)
			if assembly.Complete() {
				commit(link, store, sessions, assembly, false)
			}
		case publication.TypeQuery:
			if store.Complete(packet.ID) {
				sendSimple(link, publication.TypeComplete, packet.ID)
				continue
			}
			assembly := sessions[packet.ID]
			if assembly == nil {
				sendError(link, packet.ID, "unknown publication")
				continue
			}
			missing := assembly.Missing()
			if len(missing) > 0 {
				fmt.Printf("requesting chunks: %v\n", missing)
			}
			sendReceipt(link, assembly)
		}
	}
}

func sendReceipt(link *publication.Link, assembly *publication.Assembly) {
	packet, _ := publication.EncodeReceipt(assembly.Manifest.ID, assembly.Manifest.ChunkCount, assembly.Received())
	if err := link.Send(packet, 10*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "send receipt:", err)
	}
}

func sendSimple(link *publication.Link, kind publication.Type, id publication.ID) {
	packet, _ := publication.EncodeSimple(kind, id)
	if err := link.Send(packet, 10*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "send response:", err)
	}
}

func sendError(link *publication.Link, id publication.ID, message string) {
	packet, _ := publication.EncodeError(id, message)
	if err := link.Send(packet, 10*time.Second); err != nil {
		fmt.Fprintln(os.Stderr, "send error:", err)
	}
}

func commit(link *publication.Link, store *publication.Store, sessions map[publication.ID]*publication.Assembly, assembly *publication.Assembly, notify bool) {
	path, err := store.Commit(assembly)
	if errors.Is(err, publication.ErrHashMismatch) {
		fmt.Fprintln(os.Stderr, "sha256 mismatch; requesting all chunks again")
		restarted, restartErr := store.Restart(assembly.Manifest)
		if restartErr != nil {
			sendError(link, assembly.Manifest.ID, restartErr.Error())
			return
		}
		sessions[assembly.Manifest.ID] = restarted
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "verify/commit:", err)
		sendError(link, assembly.Manifest.ID, err.Error())
		return
	}
	fmt.Println("sha256 verified")
	fmt.Printf("publication committed: %s\n", path)
	delete(sessions, assembly.Manifest.ID)
	if notify {
		sendSimple(link, publication.TypeComplete, assembly.Manifest.ID)
	}
}
