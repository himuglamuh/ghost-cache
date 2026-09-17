package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	appconfig "github.com/himuglamuh/ghost-cache/internal/config"
	"github.com/himuglamuh/ghost-cache/internal/publication"
)

func keygen(args []string) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	dir := fs.String("data-dir", "", "node data directory")
	name := fs.String("name", "", "friendly publisher name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return errors.New("--data-dir is required")
	}
	identity, err := publication.GenerateIdentity(*dir, *name)
	if err != nil {
		return err
	}
	fmt.Printf("signing identity created\nkey_id: %s\npublic_key: %s\n", publication.KeyID(identity.PublicKey), filepath.Join(*dir, "identity", "signing.pub"))
	return nil
}
func trust(args []string) error {
	if len(args) > 0 && (args[0] == "list" || args[0] == "remove") {
		action := args[0]
		fs := flag.NewFlagSet("trust "+action, flag.ContinueOnError)
		dir := fs.String("data-dir", "", "node data directory")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if *dir == "" {
			return errors.New("--data-dir is required")
		}
		store := publication.NewTrustStore(*dir)
		if action == "list" {
			ids, err := store.IDs()
			if err != nil {
				return err
			}
			sort.Strings(ids)
			for _, id := range ids {
				fmt.Println(id)
			}
			return nil
		}
		if fs.NArg() != 1 {
			return errors.New("key ID is required")
		}
		return store.Remove(fs.Arg(0))
	}
	fs := flag.NewFlagSet("trust", flag.ContinueOnError)
	dir := fs.String("data-dir", "", "node data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || fs.NArg() != 1 {
		return errors.New("usage: ghost-node trust --data-dir DIR PUBLISHER.pub")
	}
	id, err := publication.NewTrustStore(*dir).Add(fs.Arg(0))
	if err != nil {
		return err
	}
	fmt.Printf("trusted: %s\n", id)
	return nil
}
func library(args []string) error {
	fs := flag.NewFlagSet("library", flag.ContinueOnError)
	dir := fs.String("data-dir", "", "node data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return errors.New("--data-dir is required")
	}
	store := publication.NewStore(*dir)
	entries, err := store.Catalog()
	if err != nil {
		return err
	}
	trust := publication.NewTrustStore(*dir)
	fmt.Printf("%-13s %-10s %-43s %-24s %s\n", "STATE", "SIZE", "SIGNATURE", "FILENAME", "ID")
	for _, entry := range entries {
		state := entry.State
		if state == "known" {
			state = "deferred"
		}
		if entry.Wanted {
			state += "+want"
		}
		fmt.Printf("%-13s %-10s %-43s %-24s %s\n", state, humanSize(entry.Manifest.Length), nodeSignatureStatus(entry.Manifest, trust), entry.Manifest.Filename, entry.Manifest.ID)
	}
	return nil
}
func inspect(args []string) error {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	dir := fs.String("data-dir", "", "node data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" || fs.NArg() != 1 {
		return errors.New("usage: ghost-node inspect --data-dir DIR ID_OR_PREFIX")
	}
	store := publication.NewStore(*dir)
	id, err := store.Resolve(fs.Arg(0))
	if err != nil {
		return err
	}
	entries, err := store.Catalog()
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Manifest.ID == id {
			out := map[string]any{"state": entry.State, "wanted": entry.Wanted, "publication_id": id.String(), "filename": entry.Manifest.Filename, "content_length": entry.Manifest.Length, "sha256": fmt.Sprintf("%x", entry.Manifest.SHA256), "chunk_size": entry.Manifest.ChunkSize, "chunk_count": entry.Manifest.ChunkCount, "signature": nodeSignatureStatus(entry.Manifest, publication.NewTrustStore(*dir))}
			b, _ := json.MarshalIndent(out, "", "  ")
			fmt.Println(string(b))
			return nil
		}
	}
	return errors.New("publication not found")
}
func configCommand(args []string) error {
	if len(args) == 0 || (args[0] != "check" && args[0] != "show" && args[0] != "device" && args[0] != "data-dir") {
		return errors.New("usage: ghost-node config <check|show|device|data-dir> --config FILE")
	}
	action := args[0]
	fs := flag.NewFlagSet("config "+action, flag.ContinueOnError)
	path := fs.String("config", "", "TOML configuration file")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if *path == "" {
		return errors.New("--config is required")
	}
	cfg, err := appconfig.Load(*path)
	if err != nil {
		return err
	}
	if action == "check" {
		fmt.Println("configuration valid")
		return nil
	}
	if action == "device" {
		fmt.Println(cfg.Radio.Device)
		return nil
	}
	if action == "data-dir" {
		fmt.Println(cfg.Node.DataDir)
		return nil
	}
	b, err := os.ReadFile(*path)
	if err != nil {
		return err
	}
	fmt.Print(string(b))
	return nil
}
func nodeSignatureStatus(m publication.Manifest, trust *publication.TrustStore) string {
	if m.Signature == nil {
		return "unsigned"
	}
	if publication.VerifyManifestSignature(m) != nil {
		return "invalid"
	}
	id := publication.KeyID(m.Signature.PublicKey[:])
	if trust.Trusted(m.Signature.PublicKey[:]) {
		return "trusted:" + id
	}
	return "signed/untrusted:" + id
}
func humanSize(size uint64) string {
	units := []string{"B", "KiB", "MiB", "GiB"}
	value := float64(size)
	unit := units[0]
	for i := 1; i < len(units) && value >= 1024; i++ {
		value /= 1024
		unit = units[i]
	}
	if unit == "B" {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}
