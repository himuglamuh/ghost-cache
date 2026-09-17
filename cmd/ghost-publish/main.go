package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/himuglamuh/ghost-cache/internal/publication"
)

const baud = 115200

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ghost-publish:", err)
		os.Exit(1)
	}
}

func run() error {
	device := flag.String("device", "", "modem serial device")
	file := flag.String("file", "", "file to publish")
	retries := flag.Int("retries", 8, "maximum manifest/retransmission rounds")
	responseTimeout := flag.Duration("response-timeout", 5*time.Second, "time to wait for broadcaster state")
	flag.Parse()
	if *device == "" || *file == "" {
		return errors.New("--device and --file are required")
	}
	if *retries < 1 || *responseTimeout <= 0 {
		return errors.New("--retries and --response-timeout must be positive")
	}
	content, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	if len(content) > publication.MaxChunks*publication.ChunkDataSize {
		return fmt.Errorf("file exceeds maximum size of %d bytes", publication.MaxChunks*publication.ChunkDataSize)
	}
	hash := sha256.Sum256(content)
	var id publication.ID
	copy(id[:], hash[:len(id)])
	chunkCount := 0
	if len(content) > 0 {
		chunkCount = (len(content) + publication.ChunkDataSize - 1) / publication.ChunkDataSize
	}
	manifest := publication.Manifest{ID: id, Filename: filepath.Base(*file), Length: uint64(len(content)), SHA256: hash, ChunkSize: publication.ChunkDataSize, ChunkCount: uint16(chunkCount)}
	manifestPacket, err := publication.EncodeManifest(manifest)
	if err != nil {
		return err
	}
	link, err := publication.OpenLink(*device, baud)
	if err != nil {
		return err
	}
	defer link.Close()
	fmt.Printf("publication: %s\nsize: %d bytes\nchunks: %d\n", id, len(content), chunkCount)
	received := make([]bool, chunkCount)
	for round := 1; round <= *retries; round++ {
		if round == 1 {
			fmt.Println("announcing publication...")
		}
		if err := link.Send(manifestPacket, 10*time.Second); err != nil {
			return err
		}
		packet, err := waitForState(link, id, *responseTimeout)
		if err != nil {
			fmt.Printf("manifest response timeout (%d/%d)\n", round, *retries)
			continue
		}
		if packet.Type == publication.TypeComplete {
			fmt.Println("complete: SHA-256 verified and committed")
			return nil
		}
		state, err := publication.DecodeReceipt(packet)
		if err != nil || len(state) != chunkCount {
			return errors.New("invalid receipt state from broadcaster")
		}
		copy(received, state)
		for i, have := range received {
			if have {
				continue
			}
			start := i * publication.ChunkDataSize
			end := min(start+publication.ChunkDataSize, len(content))
			chunk, _ := publication.EncodeChunk(id, uint16(i), content[start:end])
			if err := link.Send(chunk, 10*time.Second); err != nil {
				return fmt.Errorf("send chunk %d: %w", i, err)
			}
			received[i] = true
			fmt.Printf("sending: %d/%d\n", i+1, chunkCount)
		}
		query, _ := publication.EncodeSimple(publication.TypeQuery, id)
		if err := link.Send(query, 10*time.Second); err != nil {
			return err
		}
		fmt.Println("broadcaster verifying...")
		packet, err = waitForState(link, id, *responseTimeout)
		if err != nil {
			fmt.Printf("receipt timeout (%d/%d)\n", round, *retries)
			continue
		}
		if packet.Type == publication.TypeComplete {
			fmt.Println("complete: SHA-256 verified and committed")
			return nil
		}
		state, err = publication.DecodeReceipt(packet)
		if err != nil || len(state) != chunkCount {
			return errors.New("invalid receipt state from broadcaster")
		}
		copy(received, state)
		fmt.Printf("received by broadcaster: %d/%d\n", countReceived(received), chunkCount)
	}
	return fmt.Errorf("transfer did not complete after %d rounds", *retries)
}

func waitForState(link *publication.Link, id publication.ID, timeout time.Duration) (publication.Packet, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		packet, err := link.Receive(time.Until(deadline))
		if err != nil {
			return publication.Packet{}, err
		}
		if packet.ID != id {
			continue
		}
		switch packet.Type {
		case publication.TypeReceipt, publication.TypeComplete:
			return packet, nil
		case publication.TypeError:
			return publication.Packet{}, fmt.Errorf("broadcaster: %s", packet.Body)
		}
	}
	return publication.Packet{}, errors.New("timed out waiting for broadcaster")
}

func countReceived(received []bool) int {
	count := 0
	for _, ok := range received {
		if ok {
			count++
		}
	}
	return count
}
