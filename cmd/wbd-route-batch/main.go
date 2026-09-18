package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"strconv"
	"strings"
)

type batchOptions struct {
	action  string
	ifIndex uint
	nextHop netip.Addr
	metric  uint
}

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "wbd-route-batch:", err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: wbd-route-batch <add|delete> --ifindex N --next-hop IPv4 --metric N")
	}
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	s := bufio.NewScanner(in)
	s.Buffer(make([]byte, 4096), 1<<20)
	done := 0
	for lineNo := 1; s.Scan(); lineNo++ {
		raw := strings.TrimSpace(s.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || !prefix.Addr().Is4() {
			if err == nil {
				err = errors.New("not IPv4")
			}
			return fmt.Errorf("stdin line %d prefix %q: %w", lineNo, raw, err)
		}
		if err := mutateIPv4Route(opts.action, prefix.Masked(), uint32(opts.ifIndex), opts.nextHop, uint32(opts.metric)); err != nil {
			return fmt.Errorf("%s route %s: %w", opts.action, prefix.Masked(), err)
		}
		done++
		if done%500 == 0 {
			fmt.Fprintf(out, "WBD_ROUTE_BATCH_PROGRESS action=%s done=%d ifindex=%d next_hop=%s metric=%d\n", opts.action, done, opts.ifIndex, opts.nextHop, opts.metric)
		}
	}
	if err := s.Err(); err != nil {
		return err
	}
	fmt.Fprintf(out, "WBD_ROUTE_BATCH_READY action=%s routes=%d ifindex=%d next_hop=%s metric=%d\n", opts.action, done, opts.ifIndex, opts.nextHop, opts.metric)
	return nil
}

func parseOptions(args []string) (batchOptions, error) {
	action := strings.ToLower(strings.TrimSpace(args[0]))
	if action != "add" && action != "delete" {
		return batchOptions{}, fmt.Errorf("unsupported action %q", action)
	}
	fs := flag.NewFlagSet("wbd-route-batch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	ifIndex := fs.Uint("ifindex", 0, "Windows interface index")
	nextHopText := fs.String("next-hop", "", "IPv4 next hop; 0.0.0.0 for on-link")
	metric := fs.Uint("metric", 0, "route metric")
	if err := fs.Parse(args[1:]); err != nil {
		return batchOptions{}, err
	}
	if *ifIndex == 0 || *ifIndex > 0xffffffff {
		return batchOptions{}, fmt.Errorf("ifindex must be 1..%s", strconv.FormatUint(0xffffffff, 10))
	}
	if *metric > 0xffff {
		return batchOptions{}, errors.New("metric must be 0..65535")
	}
	nextHop, err := netip.ParseAddr(strings.TrimSpace(*nextHopText))
	if err != nil || !nextHop.Is4() {
		return batchOptions{}, errors.New("next-hop must be IPv4")
	}
	return batchOptions{action: action, ifIndex: *ifIndex, nextHop: nextHop, metric: *metric}, nil
}
