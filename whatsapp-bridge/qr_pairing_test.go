package main

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// TestAwaitPairingGivesUpOnASilentQRChannel reproduces Aura's lab VM on 2026-10-14. The host slept
// while a QR waited to be scanned; on waking, whatsmeow forced a reconnect (keepalive.go), and its QR
// emitter returned on the expected disconnect without closing the channel or sending a final event
// (qrchan.go). The bridge kept ranging over that channel and served the last, long-dead code for
// hours: every scan did nothing.
func TestAwaitPairingGivesUpOnASilentQRChannel(t *testing.T) {
	qrChan := make(chan whatsmeow.QRChannelItem, 1)
	qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "last-code"}
	pairing := &pairingState{}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- awaitPairing(ctx, qrChan, pairing, waLog.Noop) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a QR channel that went silent counted as a pairing")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the bridge is still waiting on a QR channel that will never speak again")
	}
	if qr, _, _, state, _ := pairing.snapshot(); qr != "" || state != "qr_timeout" {
		t.Fatalf("after giving up, the bridge serves code %q in state %q; want no code and qr_timeout", qr, state)
	}
}

// The tenant gateway copies a bridge's output into the container's log. A pairing code drawn
// there would be a live credential for its minute, readable by anything that reads the log, so the
// drawing is for a console only.
func TestAwaitPairingDrawsNoCodeIntoALog(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = stdout })

	qrChan := make(chan whatsmeow.QRChannelItem, 2)
	qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "2@secret-pairing-ref"}
	qrChan <- whatsmeow.QRChannelSuccess
	close(qrChan)
	if err := awaitPairing(context.Background(), qrChan, &pairingState{}, waLog.Noop); err != nil {
		t.Fatal(err)
	}
	os.Stdout = stdout
	_ = writer.Close()
	logged, _ := io.ReadAll(reader)

	if strings.ContainsAny(string(logged), "█▀▄") || strings.Contains(string(logged), "secret-pairing-ref") {
		t.Fatalf("the pairing code reached a log:\n%s", logged)
	}
}

// whatsmeow logs every QR code it emits at debug level (qrchan.go "Emitting QR code %s"), and
// the protocol traffic with it. Into a log the bridge writes at info.
func TestBridgeLogsAtInfoIntoALog(t *testing.T) {
	_, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = stdout })

	if got := bridgeLogLevel(); got != "INFO" {
		t.Fatalf("bridge log level into a log = %q, want INFO", got)
	}
}

func TestAwaitPairingReportsASuccessfulScan(t *testing.T) {
	qrChan := make(chan whatsmeow.QRChannelItem, 2)
	qrChan <- whatsmeow.QRChannelItem{Event: whatsmeow.QRChannelEventCode, Code: "a-code"}
	qrChan <- whatsmeow.QRChannelSuccess
	close(qrChan)

	if err := awaitPairing(context.Background(), qrChan, &pairingState{}, waLog.Noop); err != nil {
		t.Fatalf("a scanned code reported %v", err)
	}
}
