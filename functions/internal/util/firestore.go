package util

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"io"
	"log"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// TestFirestore is a handle to a Firestore emulator running as a subprocess.
//
// In order to make their use more efficient, Multiple TestFirestores may exist
// which use the same emulator under the hood. However, each will connect using
// a different project ID, so their views of the database will be
// non-overlapping.
type TestFirestore struct {
	emulator  *firestoreEmulator
	projectID string
}

// NewTestFirestore creates a new TestFirestore. It calls t.Fatal if it is not
// able to start a Firestore emulator subprocess or re-use an existing emulator
// subprocess.
func NewTestFirestore(t *testing.T) *TestFirestore {
	emul := getGlobalEmulator(t)
	var bytes [16]byte
	ReadCryptoRandBytes(bytes[:])
	return &TestFirestore{
		emulator:  emul,
		projectID: "test-" + hex.EncodeToString(bytes[:]),
	}
}

func (t *TestFirestore) clientOption() (option.ClientOption, error) {
	conn, err := grpc.Dial(t.emulator.host, grpc.WithInsecure(), grpc.WithPerRPCCredentials(emulatorCreds{}))
	if err != nil {
		return nil, err
	}
	return option.WithGRPCConn(conn), nil
}

// emulatorCreds is taken from cloud.google.com/go/firestore/client.go.
//
// TODO(https://github.com/googleapis/google-cloud-go/issues/1978): Switch to a
// first-class API if one is provided.

// emulatorCreds is an instance of grpc.PerRPCCredentials that will configure a
// client to act as an admin for the Firestore emulator. It always hardcodes
// the "authorization" metadata field to contain "Bearer owner", which the
// Firestore emulator accepts as valid admin credentials.
type emulatorCreds struct{}

func (ec emulatorCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer owner"}, nil
}

func (ec emulatorCreds) RequireTransportSecurity() bool {
	return false
}

// A global *firestoreEmulator instance for all TestFirestores to share. Use
// getGlobalEmulator to access.
var globalEmulator *firestoreEmulator

// This sync.Once initializes globalEmulator. Once it has completed, then
// globalEmulator is guaranteed to be initialized. If it is still nil, then that
// means that the initialization failed.
//
// TODO(joshlf): This solution will keep the emulator alive for the remainder of
// the process' lifetime. If we ever get into a situation in which we want to
// create an emulator, and then continue running the process for a significant
// amount of time after no code needs to access it, we should implement a
// fancier solution that detects when there are no more handles to the global
// emulator.
var globalEmulatorOnce sync.Once

func getGlobalEmulator(t *testing.T) *firestoreEmulator {
	globalEmulatorOnce.Do(func() {
		globalEmulator = newFirestoreEmulator(t)
	})

	if globalEmulator == nil {
		t.Fatal("previous attempt to initialize global emulator failed")
	}

	return globalEmulator
}

type firestoreEmulator struct {
	// The emulator subprocess.
	emulator *exec.Cmd
	host     string
}

func newFirestoreEmulator(t *testing.T) *firestoreEmulator {
	// For some reason, if the emulator crashes, the stderr does not close, and
	// without any other intervention, our stderr scanning code hangs
	// indefinitely. This timeout ensures that, if that happens, this timeout
	// will eventually trigger, the process will be killed (thus closing
	// stderr), and we will return an error.
	//
	// TODO(joshlf): Implement a more sophisticated solution to this problem.
	// The right way to do this is probably to spawn two goroutines - one to
	// scan stderr, and one to monitor the process to see if it quits.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Do this separately ahead of time (rather than just passing "gcloud" to
	// exec.CommandContext) to give more precise errors.
	gcloudPath, err := exec.LookPath("gcloud")
	if err != nil {
		t.Fatalf("could not find gcloud: %v", err)
	}
	// When we don't specify a local address, a random port is chosen
	// automatically.
	cmd := exec.CommandContext(ctx, gcloudPath, "beta", "emulators", "firestore", "start")
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("could not start Firestore emulator: %v", err)
	}
	defer stderr.Close()

	if err := cmd.Start(); err != nil {
		t.Fatalf("could not start Firestore emulator: %v", err)
	}

	emul := &firestoreEmulator{emulator: cmd}
	// Set a finalizer so that the subprocess is killed when we're done with it.
	runtime.SetFinalizer(emul, func(emul *firestoreEmulator) {
		err := emul.emulator.Process.Kill()
		if err != nil {
			log.Print("could not kill emulator process:", err)
		}
	})

	// Parse the command's stderr until we find the line indicating what local
	// address the emulator is listening on. Example output:
	//
	//  Executing: /Users/dvader/google-cloud-sdk/platform/cloud-firestore-emulator/cloud_firestore_emulator start --host=::1 --port=8007
	//  [firestore] API endpoint: http://::1:8007
	//  [firestore] If you are using a library that supports the FIRESTORE_EMULATOR_HOST environment variable, run:
	//  [firestore]
	//  [firestore]    export FIRESTORE_EMULATOR_HOST=::1:8007
	//  [firestore]
	//  [firestore] Dev App Server is now running.
	//  [firestore]
	//
	// In particular, we look for "export FIRESTORE_EMULATOR_HOST=".

	// Tee all of stderr to a buffer so we can provide more informative error
	// messages if need be.
	cached := bytes.NewBuffer(nil)
	s := bufio.NewScanner(io.TeeReader(stderr, cached))
	var host string
loop:
	for s.Scan() {
		parts := strings.Split(s.Text(), "export FIRESTORE_EMULATOR_HOST=")
		switch len(parts) {
		case 1:
			if strings.Contains(s.Text(), "Dev App Server is now running") {
				break loop
			}
		case 2:
			host = parts[1]
		default:
			t.Fatalf("got unexpected line from stderr output: \"%v\"", s.Text())
		}
	}

	if err := s.Err(); err != nil {
		t.Fatalf("error reading output: %v; contents of stderr:\n%v", err, string(cached.Bytes()))
	}

	if host == "" {
		t.Fatalf("emulator started without outputting its listening address; contents of stderr:\n%s", string(cached.Bytes()))
	}

	// Instead of just storing the host as is, we split it into two and then
	// recombine it using net.JoinHostPort. The reason is that, when using IPv6,
	// the host looks like "::1:8007", which cannot be parsed as an IP
	// address/port pair by the Go Firestore client. net.JoinHostPort takes care
	// of wrapping IPv6 addresses in square brackets.
	colon := strings.LastIndex(host, ":")
	if colon == -1 {
		t.Fatalf("could not parse host: %v", host)
	}

	emul.host = net.JoinHostPort(host[:colon], host[colon+1:])
	return emul
}
