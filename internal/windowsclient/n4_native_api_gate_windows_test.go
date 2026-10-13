//go:build windows

package windowsclient

import (
    "testing"

    "golang.org/x/sys/windows"
)

// This is a NON-ELEVATED capability probe, not network mutation.
// N4's PowerShell-free networking backend must use Windows DLL calls,
// and fail closed if the installed Windows edition lacks a required API.
func TestN4NativeNetworkDLLExportsAvailable(t *testing.T) {
    groups := []struct{
        dll string
        procedures []string
    }{
        {"iphlpapi.dll", []string{
            "GetIpForwardTable2", "CreateIpForwardEntry2", "DeleteIpForwardEntry2",
            "InitializeIpForwardEntry", "GetUnicastIpAddressTable",
            "GetUnicastIpAddressEntry", "CreateUnicastIpAddressEntry",
            "DeleteUnicastIpAddressEntry", "InitializeUnicastIpAddressEntry",
            "GetIpInterfaceEntry", "SetIpInterfaceEntry",
        }},
        {"fwpuclnt.dll", []string{
            "FwpmEngineOpen0", "FwpmEngineClose0", "FwpmSubLayerAdd0",
            "FwpmFilterAdd0", "FwpmFilterDeleteById0",
        }},
    }
    for _, group := range groups {
        t.Run(group.dll, func(t *testing.T) {
            dll := windows.NewLazySystemDLL(group.dll)
            if err := dll.Load(); err != nil {
                t.Fatal(err)
            }
            for _, name := range group.procedures {
                if err := dll.NewProc(name).Find(); err != nil {
                    t.Errorf("required Windows native API %s!%s unavailable: %v", group.dll, name, err)
                }
            }
        })
    }
}
