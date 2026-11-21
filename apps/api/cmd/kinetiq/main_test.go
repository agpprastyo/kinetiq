package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strings"
	"testing"
)

func TestVersionArgs(t *testing.T) {
	for _, arg := range []string{"version", "--version", "-v"} {
		if !isVersionArg(arg) {
			t.Errorf("isVersionArg(%q) = false, want true", arg)
		}
	}
	for _, arg := range []string{"backup", "-version", "ver", ""} {
		if isVersionArg(arg) {
			t.Errorf("isVersionArg(%q) = true, want false", arg)
		}
	}
}

func TestPrintVersion(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })

	var buf bytes.Buffer
	printVersion(&buf)
	if got := buf.String(); got != "kinetiq dev\n" {
		t.Errorf("default version output = %q, want %q", got, "kinetiq dev\n")
	}

	Version = "v1.2.3"
	buf.Reset()
	printVersion(&buf)
	if got := buf.String(); got != "kinetiq v1.2.3\n" {
		t.Errorf("overridden version output = %q, want %q", got, "kinetiq v1.2.3\n")
	}
}

func TestBackup_SHA256MatchesWrittenArchive(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working dir: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir to temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origDir)
	})

	envContent := "FOO=bar\nAPP_ENV=test\n"
	if err := os.WriteFile(".env", []byte(envContent), 0600); err != nil {
		t.Fatalf("failed to write .env: %v", err)
	}

	// Capture stdout during backup()
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = origStdout
	})

	backup()

	_ = w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	_ = r.Close()

	output := buf.String()
	var backupFile, printedSHA string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Backup created: ") {
			backupFile = strings.TrimPrefix(line, "Backup created: ")
		}
		if strings.HasPrefix(line, "SHA256: ") {
			printedSHA = strings.TrimPrefix(line, "SHA256: ")
		}
	}

	if backupFile == "" {
		t.Fatalf("expected 'Backup created: <file>' in output, got:\n%s", output)
	}
	if printedSHA == "" {
		t.Fatalf("expected 'SHA256: <hash>' in output, got:\n%s", output)
	}

	archiveBytes, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatalf("failed to read backup archive %s: %v", backupFile, err)
	}

	actualSum := sha256.Sum256(archiveBytes)
	actualSHA := hex.EncodeToString(actualSum[:])
	if printedSHA != actualSHA {
		t.Fatalf("printed SHA256 mismatch: printed=%s actual=%s", printedSHA, actualSHA)
	}

	// Verify archive is a valid tar.gz containing .env
	gr, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		t.Fatalf("archive is not valid gzip: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("failed to read tar header: %v", err)
	}
	if hdr.Name != ".env" {
		t.Errorf("expected archive entry '.env', got %q", hdr.Name)
	}
	extracted, err := io.ReadAll(tr)
	if err != nil {
		t.Fatalf("failed to read .env from archive: %v", err)
	}
	if string(extracted) != envContent {
		t.Errorf("extracted .env = %q, want %q", string(extracted), envContent)
	}
}
