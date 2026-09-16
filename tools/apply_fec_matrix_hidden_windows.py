from pathlib import Path


def replace_once(path: str, old: str, new: str):
    p = Path(path)
    text = p.read_text(encoding='utf-8')
    n = text.count(old)
    if n != 1:
        raise SystemExit(f'{path}: expected one occurrence, got {n}: {old[:120]!r}')
    p.write_text(text.replace(old, new, 1), encoding='utf-8')


# Extend the proven 20x20 RS superset to the historical fixed-repair levels.
# The data geometry remains 20 systematic shards; each profile transmits the
# first R MDS repair rows. This avoids introducing a second incompatible codec.
replace_once('internal/fec/fec.go',
'''func validParityCount(n int) bool { return n == WeakParityShards || n == ParityShards }''',
'''func validParityCount(n int) bool {
\tswitch n {
\tcase 4, 8, WeakParityShards, 12, 16, ParityShards:
\t\treturn true
\tdefault:
\t\treturn false
\t}
}''')

replace_once('internal/control/link.go',
'''\t\tAllowedFixedFEC: []FixedFECProfile{
\t\t\t{DataShards: 20, ParityShards: 10, Scheduler: FECSchedulerTailRS},
\t\t\t{DataShards: 20, ParityShards: 20, Scheduler: FECSchedulerTailRS},
\t\t},''',
'''\t\tAllowedFixedFEC: []FixedFECProfile{
\t\t\t{DataShards: 20, ParityShards: 4, Scheduler: FECSchedulerTailRS},
\t\t\t{DataShards: 20, ParityShards: 8, Scheduler: FECSchedulerTailRS},
\t\t\t{DataShards: 20, ParityShards: 10, Scheduler: FECSchedulerTailRS},
\t\t\t{DataShards: 20, ParityShards: 12, Scheduler: FECSchedulerTailRS},
\t\t\t{DataShards: 20, ParityShards: 16, Scheduler: FECSchedulerTailRS},
\t\t\t{DataShards: 20, ParityShards: 20, Scheduler: FECSchedulerTailRS},
\t\t},''')

replace_once('internal/linkdata/path.go',
'''var ErrUnsupportedLinkConfig = errors.New("linkdata: unsupported immutable link config")
''',
'''var ErrUnsupportedLinkConfig = errors.New("linkdata: unsupported immutable link config")

func supportedFixedParityShards(n uint8) bool {
\tswitch n {
\tcase 4, 8, 10, 12, 16, 20:
\t\treturn true
\tdefault:
\t\treturn false
\t}
}
''')
replace_once('internal/linkdata/path.go',
'''\tif config.FECMode != control.FECFixed ||
\t\tconfig.Scheduler != control.FECSchedulerTailRS ||
\t\tconfig.DataShards != fec.DataShards ||
\t\t(config.ParityShards != fec.WeakParityShards && config.ParityShards != fec.ParityShards) {''',
'''\tif config.FECMode != control.FECFixed ||
\t\tconfig.Scheduler != control.FECSchedulerTailRS ||
\t\tconfig.DataShards != fec.DataShards ||
\t\t!supportedFixedParityShards(config.ParityShards) {''')

replace_once('cmd/wbd-link-proxy/main.go',
'''\tflag.StringVar(&o.fec, "fec", "20:20", "client immutable FEC profile: off or 20:20")''',
'''\tflag.StringVar(&o.fec, "fec", "20:20", "client immutable FEC profile: off, 20:4, 20:8, 20:10, 20:12, 20:16, or 20:20")''')
replace_once('cmd/wbd-link-proxy/main.go',
'''\tcase "20:20", "weak-2x":
\t\tif o.flushMS <= 0 {
\t\t\treturn control.LinkConfig{}, errors.New("20:20 requires positive -fec-flush-ms")
\t\t}
\t\tcfg.FECMode = control.FECFixed
\t\tcfg.Scheduler = control.FECSchedulerTailRS
\t\tcfg.DataShards = 20
\t\tcfg.ParityShards = 20
\t\tcfg.FlushMillis = uint16(o.flushMS)
\tcase "20:10", "weak-1.5x":
\t\tif o.flushMS <= 0 {
\t\t\treturn control.LinkConfig{}, errors.New("20:10 requires positive -fec-flush-ms")
\t\t}
\t\tcfg.FECMode = control.FECFixed
\t\tcfg.Scheduler = control.FECSchedulerTailRS
\t\tcfg.DataShards = 20
\t\tcfg.ParityShards = 10
\t\tcfg.FlushMillis = uint16(o.flushMS)''',
'''\tcase "20:4", "20:8", "20:10", "20:12", "20:16", "20:20", "weak-1.5x", "weak-2x":
\t\tif o.flushMS <= 0 {
\t\t\treturn control.LinkConfig{}, fmt.Errorf("%s requires positive -fec-flush-ms", o.fec)
\t\t}
\t\tparity := map[string]uint8{
\t\t\t"20:4": 4, "20:8": 8, "20:10": 10, "20:12": 12, "20:16": 16, "20:20": 20,
\t\t\t"weak-1.5x": 10, "weak-2x": 20,
\t\t}[o.fec]
\t\tcfg.FECMode = control.FECFixed
\t\tcfg.Scheduler = control.FECSchedulerTailRS
\t\tcfg.DataShards = 20
\t\tcfg.ParityShards = parity
\t\tcfg.FlushMillis = uint16(o.flushMS)''')

replace_once('internal/windowsruntime/plan.go',
'''\tif p.FEC != "off" && p.FEC != "20:10" && p.FEC != "20:20" {
\t\treturn errors.New("FEC must be off, 20:10, or 20:20")
\t}''',
'''\tswitch p.FEC {
\tcase "off", "20:4", "20:8", "20:10", "20:12", "20:16", "20:20":
\tdefault:
\t\treturn errors.New("FEC must be off, 20:4, 20:8, 20:10, 20:12, 20:16, or 20:20")
\t}''')
replace_once('internal/windowsruntime/game_mtu.go',
'''\tcase "20:10", "20:20":
\t\tfecEnabled = true''',
'''\tcase "20:4", "20:8", "20:10", "20:12", "20:16", "20:20":
\t\tfecEnabled = true''')

# Windows child processes must never surface console windows when launched by
# the GUI/runtime, including the remaining PowerShell compatibility commands.
replace_once('internal/windowsruntime/executor.go',
'''func (OSRunner) Run(command Command) error {cmd:=exec.Command(command.Path,command.Args...);cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;return cmd.Run()}
func (OSRunner) Start(command Command)(Process,error){cmd:=exec.Command(command.Path,command.Args...);out:=newProcessOutput();''',
'''func (OSRunner) Run(command Command) error {cmd:=exec.Command(command.Path,command.Args...);configureHiddenProcess(cmd);cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;return cmd.Run()}
func (OSRunner) Start(command Command)(Process,error){cmd:=exec.Command(command.Path,command.Args...);configureHiddenProcess(cmd);out:=newProcessOutput();''')
replace_once('internal/windowsruntime/controller.go',
'''\tcmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", "Status")
\toutput, err := cmd.CombinedOutput()''',
'''\tcmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", "Status")
\tconfigureHiddenProcess(cmd)
\toutput, err := cmd.CombinedOutput()''')
replace_once('internal/windowsruntime/controller.go',
'''\tcmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-RemoteIPAddress", raw.Addr().String())
\toutput, err := cmd.CombinedOutput()''',
'''\tcmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-RemoteIPAddress", raw.Addr().String())
\tconfigureHiddenProcess(cmd)
\toutput, err := cmd.CombinedOutput()''')

# Direct GUI/launcher commands (Npcap setup and GUI handoff) are outside the
# runtime Runner and receive the same no-console policy explicitly.
replace_once('cmd/wbd-windows-gui/main_windows.go',
'''\t\tcmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", "Install")
\t\tout, runErr := cmd.CombinedOutput()''',
'''\t\tcmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", "Install")
\t\tcmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
\t\tout, runErr := cmd.CombinedOutput()''')
replace_once('cmd/wbd-windows-portable/main_windows.go',
'''\t\tcmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", "Install")
\t\tif output, err := cmd.CombinedOutput(); err != nil {''',
'''\t\tcmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "-Action", "Install")
\t\tcmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
\t\tif output, err := cmd.CombinedOutput(); err != nil {''')
replace_once('cmd/wbd-windows-portable/main_windows.go',
'''\tcmd := exec.Command(gui, args...)
\tcmd.Dir = portableDir''',
'''\tcmd := exec.Command(gui, args...)
\tcmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
\tcmd.Dir = portableDir''')

Path('internal/windowsruntime/process_hidden_windows.go').write_text(r'''//go:build windows

package windowsruntime

import (
    "os/exec"
    "syscall"
)

const createNoWindow = 0x08000000

func configureHiddenProcess(cmd *exec.Cmd) {
    if cmd == nil {
        return
    }
    cmd.SysProcAttr = &syscall.SysProcAttr{
        HideWindow: true,
        CreationFlags: createNoWindow,
    }
}
''', encoding='utf-8')
Path('internal/windowsruntime/process_hidden_other.go').write_text(r'''//go:build !windows

package windowsruntime

import "os/exec"

func configureHiddenProcess(cmd *exec.Cmd) {}
''', encoding='utf-8')

Path('internal/fec/profile_matrix_test.go').write_text(r'''package fec

import (
    "bytes"
    "fmt"
    "testing"
    "time"
)

func TestFixedParityProfileMatrixRecoversEquivalentSystematicLoss(t *testing.T) {
    for _, parity := range []int{4, 8, 10, 12, 16, 20} {
        t.Run(fmt.Sprintf("20_%d", parity), func(t *testing.T) {
            codec := NewFastReedSolomon20x20()
            enc, err := NewFastBlockEncoderWithParity(codec, 1400, 8*time.Millisecond, 77, parity)
            if err != nil { t.Fatal(err) }
            dec, err := NewBlockDecoderWithParity(NewFastReedSolomon20x20(), 1400, 4, parity)
            if err != nil { t.Fatal(err) }

            want := make([][]byte, DataShards)
            var wire [][]byte
            now := time.Unix(1, 0)
            for i := 0; i < DataShards; i++ {
                want[i] = []byte{byte(i), byte(255-i), byte(i ^ 0x5a), byte(i + 17)}
                out, err := enc.Add(want[i], now)
                if err != nil { t.Fatal(err) }
                for _, datagram := range out {
                    wire = append(wire, append([]byte(nil), datagram...))
                }
            }
            if got, wantN := len(wire), DataShards+parity; got != wantN {
                t.Fatalf("wire shards=%d want=%d", got, wantN)
            }
            for _, datagram := range wire {
                h, err := ParseBlockHeader(datagram[:HeaderSize])
                if err != nil { t.Fatal(err) }
                if h.EffectiveParityCount() != parity {
                    t.Fatalf("wire parity=%d want=%d", h.EffectiveParityCount(), parity)
                }
            }

            got := make(map[byte][]byte, DataShards)
            // Drop exactly R systematic shards. The remaining 20-R sources plus
            // R repair rows must reconstruct all R missing originals.
            for _, datagram := range wire {
                h, err := ParseBlockHeader(datagram[:HeaderSize])
                if err != nil { t.Fatal(err) }
                if int(h.ShardIndex) < parity {
                    continue
                }
                packets, _, err := dec.Add(datagram)
                if err != nil { t.Fatal(err) }
                for _, packet := range packets {
                    got[packet[0]] = packet
                }
            }
            if len(got) != DataShards {
                t.Fatalf("recovered packets=%d want=%d", len(got), DataShards)
            }
            for i, packet := range want {
                if !bytes.Equal(got[byte(i)], packet) {
                    t.Fatalf("packet %d mismatch: got=%v want=%v", i, got[byte(i)], packet)
                }
            }
        })
    }
}

func TestRejectsNonProductParityGeometry(t *testing.T) {
    for _, parity := range []int{1, 5, 6, 9, 11, 13, 19} {
        if _, err := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1400, time.Millisecond, 1, parity); err == nil {
            t.Fatalf("parity=%d unexpectedly accepted", parity)
        }
    }
}
''', encoding='utf-8')

Path('internal/control/fec_profiles_test.go').write_text(r'''package control

import (
    "errors"
    "testing"
)

func TestCurrentLinkPolicyAdmitsFixedParityMatrix(t *testing.T) {
    p := CurrentLinkPolicy()
    for _, parity := range []uint8{4, 8, 10, 12, 16, 20} {
        cfg := fixed20x20Link()
        cfg.ParityShards = parity
        if err := p.Validate(cfg); err != nil {
            t.Fatalf("20:%d rejected: %v", parity, err)
        }
    }
    cfg := fixed20x20Link()
    cfg.ParityShards = 9
    if err := p.Validate(cfg); !errors.Is(err, ErrUnsupported) {
        t.Fatalf("20:9 err=%v want ErrUnsupported", err)
    }
}
''', encoding='utf-8')

Path('cmd/wbd-link-proxy/fec_profiles_test.go').write_text(r'''package main

import (
    "fmt"
    "testing"
)

func TestClientLinkConfigFixedParityMatrix(t *testing.T) {
    for _, parity := range []int{4, 8, 10, 12, 16, 20} {
        name := fmt.Sprintf("20:%d", parity)
        cfg, err := clientLinkConfig(options{fec: name, mtu: 1400, flushMS: 8, lanes: 1})
        if err != nil { t.Fatalf("%s: %v", name, err) }
        if int(cfg.DataShards) != 20 || int(cfg.ParityShards) != parity {
            t.Fatalf("%s -> %d:%d", name, cfg.DataShards, cfg.ParityShards)
        }
    }
}
''', encoding='utf-8')

Path('internal/windowsruntime/fec_profiles_test.go').write_text(r'''package windowsruntime

import (
    "fmt"
    "strings"
    "testing"
)

func validFECProfileForTest(fec string) Profile {
    return Profile{
        BinDir: `C:\\wbd`, ServerFront: "203.0.113.10:443", ServerName: "example.test",
        RouteKey: strings.Repeat("k", 16), Username: "u", Password: "p", ServerRaw: "203.0.113.10:443",
        FEC: fec, MTU: 1500, RouteMode: RouteFull, DNSMode: DNSSystem,
        InstallationID: strings.Repeat("a", 32), TicketPath: `C:\\state\\ticket`,
        TunnelConfigPath: `C:\\state\\tunnel.json`, RouteState: `C:\\state\\route.json`,
    }
}

func TestProfileAcceptsFixedFECMatrix(t *testing.T) {
    for _, parity := range []int{4, 8, 10, 12, 16, 20} {
        fec := fmt.Sprintf("20:%d", parity)
        if err := validFECProfileForTest(fec).Validate(); err != nil {
            t.Fatalf("%s: %v", fec, err)
        }
        if _, err := gameConnectionMTUBudget(1500, fec); err != nil {
            t.Fatalf("budget %s: %v", fec, err)
        }
    }
    if err := validFECProfileForTest("20:9").Validate(); err == nil {
        t.Fatal("20:9 unexpectedly accepted")
    }
}
''', encoding='utf-8')

Path('internal/windowsruntime/process_hidden_windows_test.go').write_text(r'''//go:build windows

package windowsruntime

import (
    "os/exec"
    "testing"
)

func TestConfigureHiddenProcessUsesNoConsoleWindow(t *testing.T) {
    cmd := exec.Command("cmd.exe", "/c", "exit", "0")
    configureHiddenProcess(cmd)
    if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
        t.Fatal("HideWindow was not enabled")
    }
    if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
        t.Fatalf("CreationFlags=%#x missing CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
    }
}
''', encoding='utf-8')

print('FEC matrix + hidden Windows process patch applied')
