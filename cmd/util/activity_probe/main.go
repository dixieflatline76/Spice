// activity_probe is a developer utility that exercises the Activity Gate's
// platform detectors in real time on hardware.
//
// Usage:
//
//	activity_probe             # one-shot snapshot of all detection signals
//	activity_probe -watch      # 1s polling loop (for alt-tabbing into games)
//	activity_probe -gpu        # print per-process 3D-engine GPU utilization (Windows)
//	activity_probe -allow "cs2.exe, eldenring.exe"  # test allowlist matching
//
// Part of Milestone 3.0 (Engine Intelligence). See plans/activity-gate-design.md.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/dixieflatline76/Spice/v2/pkg/activity"
)

func main() {
	watch := flag.Bool("watch", false, "poll detection signals once per second")
	gpuOnly := flag.Bool("gpu", false, "show raw 3D-engine GPU load per process")
	allowlist := flag.String("allow", "", "comma-separated process allowlist to test (S3)")
	flag.Parse()

	det := activity.New()
	if det == nil {
		fmt.Fprintln(os.Stderr, "error: platform returned nil Detector")
		os.Exit(1)
	}

	if *gpuOnly {
		runGpuProbe(det)
		return
	}

	if *watch {
		runWatchLoop(det, *allowlist)
		return
	}

	printSnapshot(det, *allowlist)
}

func printSnapshot(det activity.Detector, allowlist string) {
	fmt.Println("=== Spice Activity Gate — Detection Snapshot ===")

	busy := det.UserBusy()
	fmt.Printf("S1/S2 Combined busy state : %s (%d)\n", busy, busy)

	fg, ok := det.ForegroundProcess()
	if ok {
		hit := activity.MatchesProcessList(fg, allowlist)
		fmt.Printf("S3    Foreground process : %q (allowlist hit=%t)\n", fg, hit)
	} else {
		fmt.Println("S3    Foreground process : <unavailable>")
	}

	batt := det.Battery()
	fmt.Printf("Power Battery state       : %s (low@20%%=%t)\n", batt, batt.Low(20))

	if gd, ok := det.(activity.GpuDetector); ok {
		loads, gOk := gd.GpuLoad3D()
		if gOk {
			top := topGpuProcesses(loads, 3)
			fmt.Printf("S5    Top 3D GPU load     : %s\n", formatGpuTop(top))
		} else {
			fmt.Println("S5    3D GPU load         : <unavailable>")
		}
	} else {
		fmt.Println("S5    3D GPU load         : <unsupported on this OS>")
	}
}

func runWatchLoop(det activity.Detector, allowlist string) {
	fmt.Println("Polling every 1s. Alt-tab into your game/presentation to test. Ctrl+C to stop.")
	fmt.Println("Timestamp | Busy       | Battery | Foreground process           | Allowlist | GPU 3D top")
	fmt.Println("----------+------------+---------+------------------------------+-----------+-----------")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Println("\nStopped.")
			return
		case t := <-ticker.C:
			busy := det.UserBusy()
			batt := det.Battery()
			fg, _ := det.ForegroundProcess()
			hit := activity.MatchesProcessList(fg, allowlist)

			gpuStr := "-"
			if gd, ok := det.(activity.GpuDetector); ok {
				if loads, gOk := gd.GpuLoad3D(); gOk {
					top := topGpuProcesses(loads, 1)
					if len(top) > 0 {
						gpuStr = fmt.Sprintf("pid %d: %.0f%%", top[0].pid, top[0].pct)
					}
				}
			}

			fgDisplay := fg
			if len(fgDisplay) > 28 {
				fgDisplay = "..." + fgDisplay[len(fgDisplay)-25:]
			}

			fmt.Printf("%s  | %-10s | %-7s | %-28s | %-9t | %s\n",
				t.Format("15:04:05"),
				busy,
				batt,
				fgDisplay,
				hit,
				gpuStr,
			)
		}
	}
}

func runGpuProbe(det activity.Detector) {
	gd, ok := det.(activity.GpuDetector)
	if !ok {
		fmt.Fprintln(os.Stderr, "GPU detection not supported on this platform")
		os.Exit(1)
	}
	fmt.Println("Sampling 3D-engine GPU utilization across all processes (PDH)...")
	loads, ok := gd.GpuLoad3D()
	if !ok {
		fmt.Fprintln(os.Stderr, "GpuLoad3D returned ok=false (counters unavailable or driver error)")
		os.Exit(2)
	}
	top := topGpuProcesses(loads, 15)
	if len(top) == 0 {
		fmt.Println("No processes showing 3D-engine utilization.")
		return
	}
	fmt.Printf("%-8s | %s\n", "PID", "3D GPU %")
	fmt.Println("---------+---------")
	for _, p := range top {
		fmt.Printf("%-8d | %6.1f%%\n", p.pid, p.pct)
	}
}

type pidPct struct {
	pid int
	pct float64
}

func topGpuProcesses(loads map[int]float64, n int) []pidPct {
	var list []pidPct
	for pid, pct := range loads {
		list = append(list, pidPct{pid: pid, pct: pct})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].pct > list[j].pct
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

func formatGpuTop(top []pidPct) string {
	if len(top) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(top))
	for _, p := range top {
		parts = append(parts, fmt.Sprintf("pid %d: %.0f%%", p.pid, p.pct))
	}
	return strings.Join(parts, ", ")
}
