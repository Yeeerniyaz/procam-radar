package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/tls"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const Version = "4.1.0"
const AppName = "ProCam Radar"

var cameraOUI = map[string]string{
	"04:ee:cd": "Hikvision / HiWatch", "08:3a:2f": "Hikvision / HiWatch", "08:54:11": "Hikvision / HiWatch",
	"08:a1:89": "Hikvision / HiWatch", "08:cc:81": "Hikvision / HiWatch", "10:12:fb": "Hikvision / HiWatch",
	"14:2f:fd": "Hikvision / HiWatch", "18:68:cb": "Hikvision / HiWatch", "18:80:25": "Hikvision / HiWatch",
	"24:0f:9b": "Hikvision / HiWatch", "24:28:fd": "Hikvision / HiWatch", "24:32:ae": "Hikvision / HiWatch",
	"24:48:45": "Hikvision / HiWatch", "28:57:be": "Hikvision / HiWatch", "2c:a5:9c": "Hikvision / HiWatch",
	"34:09:62": "Hikvision / HiWatch", "3c:1b:f8": "Hikvision / HiWatch", "40:ac:bf": "Hikvision / HiWatch",
	"44:19:b6": "Hikvision / HiWatch", "44:47:cc": "Hikvision / HiWatch", "44:a6:42": "Hikvision / HiWatch",
	"4c:1f:86": "Hikvision / HiWatch", "4c:62:df": "Hikvision / HiWatch", "4c:bd:8f": "Hikvision / HiWatch",
	"4c:f5:dc": "Hikvision / HiWatch", "50:e5:38": "Hikvision / HiWatch", "54:8c:81": "Hikvision / HiWatch",
	"54:c4:15": "Hikvision / HiWatch", "58:03:fb": "Hikvision / HiWatch", "58:50:ed": "Hikvision / HiWatch",
	"5c:34:5b": "Hikvision / HiWatch", "64:db:8b": "Hikvision / HiWatch", "68:6d:bc": "Hikvision / HiWatch",
	"74:3f:c2": "Hikvision / HiWatch", "80:48:9f": "Hikvision / HiWatch", "80:7c:62": "Hikvision / HiWatch",
	"80:be:af": "Hikvision / HiWatch", "80:f5:ae": "Hikvision / HiWatch", "84:94:59": "Hikvision / HiWatch",
	"84:9a:40": "Hikvision / HiWatch", "88:de:39": "Hikvision / HiWatch", "8c:e7:48": "Hikvision / HiWatch",
	"94:e1:ac": "Hikvision / HiWatch", "98:8b:0a": "Hikvision / HiWatch", "98:9d:e5": "Hikvision / HiWatch",
	"98:df:82": "Hikvision / HiWatch", "9c:f6:dd": "Hikvision / HiWatch", "a0:ff:0c": "Hikvision / HiWatch",
	"a4:14:37": "Hikvision / HiWatch", "a4:29:02": "Hikvision / HiWatch", "a4:4b:d5": "Hikvision / HiWatch",
	"a4:a4:59": "Hikvision / HiWatch", "a4:d5:c2": "Hikvision / HiWatch", "ac:b9:2f": "Hikvision / HiWatch",
	"ac:cb:51": "Hikvision / HiWatch", "b4:a3:82": "Hikvision / HiWatch", "bc:5e:33": "Hikvision / HiWatch",
	"bc:9b:5e": "Hikvision / HiWatch", "bc:ad:28": "Hikvision / HiWatch", "bc:ba:c2": "Hikvision / HiWatch",
	"c0:51:7e": "Hikvision / HiWatch", "c0:56:e3": "Hikvision / HiWatch", "c0:6d:ed": "Hikvision / HiWatch",
	"c4:2f:90": "Hikvision / HiWatch", "c8:02:8f": "Hikvision / HiWatch", "cc:8c:bf": "Hikvision / HiWatch",
	"d4:e8:53": "Hikvision / HiWatch", "dc:07:f8": "Hikvision / HiWatch", "dc:d2:6a": "Hikvision / HiWatch",
	"e0:ba:ad": "Hikvision / HiWatch", "e0:ca:3c": "Hikvision / HiWatch", "e0:df:13": "Hikvision / HiWatch",
	"e4:d5:8b": "Hikvision / HiWatch", "e8:a0:ed": "Hikvision / HiWatch", "ec:a9:71": "Hikvision / HiWatch",
	"ec:c8:9c": "Hikvision / HiWatch", "f8:4d:fc": "Hikvision / HiWatch", "fc:9f:fd": "Hikvision / HiWatch",
	"00:12:41": "Dahua", "08:ed:ed": "Dahua", "14:a7:8b": "Dahua", "18:2a:7b": "Dahua", "24:52:6a": "Dahua",
	"2c:f0:5d": "Dahua", "30:dd:aa": "Dahua", "38:af:29": "Dahua", "3c:ef:8c": "Dahua", "4c:11:bf": "Dahua",
	"5c:a6:e6": "Dahua", "5c:f2:07": "Dahua", "64:f2:fb": "Dahua", "6c:1c:71": "Dahua", "70:c9:76": "Dahua",
	"74:a7:8e": "Dahua", "84:e9:84": "Dahua", "90:02:a9": "Dahua", "9c:14:63": "Dahua", "a0:bd:1d": "Dahua",
	"b0:60:88": "Dahua", "bc:32:5f": "Dahua", "c0:39:5a": "Dahua", "d4:43:0e": "Dahua", "e0:50:8b": "Dahua",
	"e4:24:6c": "Dahua", "ec:71:db": "Dahua", "f0:41:c8": "Dahua", "f8:ce:07": "Dahua", "fc:5f:49": "Dahua",
	"00:1c:27": "Tiandy", "18:68:82": "Tiandy", "30:49:50": "Tiandy", "40:6a:8e": "Tiandy", "6c:5c:3d": "Tiandy",
	"70:66:55": "Tiandy", "84:5d:d7": "Tiandy", "9c:41:7c": "Tiandy", "c4:34:6b": "Tiandy", "d8:76:0a": "Tiandy",
	"48:ea:63": "Uniview (UNV)", "6c:f1:7e": "Uniview (UNV)", "00:12:12": "Xiongmai (XM)", "00:40:8c": "Axis",
}

type CameraEntry struct {
	IP          string `json:"ip"`
	MAC         string `json:"mac"`
	Vendor      string `json:"vendor"`
	Model       string `json:"model"`
	Serial      string `json:"serial"`
	Firmware    string `json:"firmware"`
	DeviceType  string `json:"device_type"`  // "nvr" or "camera"
	DeviceLabel string `json:"device_label"` // "NVR / Регистратор" or "IP-Камера"
	Ports       []int  `json:"ports"`
	Zone        string `json:"zone"`
	ZoneType    string `json:"zone_type"`
	Iface       string `json:"iface"`
	IfaceKind   string `json:"iface_kind"`
	SubnetAlias string `json:"subnet_alias"`
	WebLink     string `json:"web_link"`
	RTSPLink    string `json:"rtsp_link"`
	Conflict    bool   `json:"conflict"`
}

type NetInterfaceInfo struct {
	Name    string     `json:"name"`
	Kind    string     `json:"kind"` // "wired" or "wifi"
	Label   string     `json:"label"`
	IP      string     `json:"ip"`
	Subnet  string     `json:"subnet"`
	Carrier bool       `json:"carrier"`
	IPNet   *net.IPNet `json:"-"`
}

var (
	stateMu         sync.Mutex
	cachedCameras   []CameraEntry
	injectedSubnets = make(map[string]string)
	injectedRoutes  = make(map[string]string)
	appTempDir      string
	serverPort      int
	publicTunnelURL string
)

func runHiddenCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	prepareBackgroundCmd(cmd)
	return cmd.Run()
}

func runHiddenOutput(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	prepareBackgroundCmd(cmd)
	return cmd.CombinedOutput()
}

func runPrivilegedLinux(args ...string) error {
	if len(args) == 0 {
		return nil
	}
	if os.Geteuid() == 0 {
		return runHiddenCmd(args[0], args[1:]...)
	}
	sudoArgs := append([]string{"-n"}, args...)
	if err := runHiddenCmd("sudo", sudoArgs...); err == nil {
		return nil
	}
	return runHiddenCmd(args[0], args[1:]...)
}

func lookupVendor(mac string) string {
	m := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(mac), "-", ":"))
	if len(m) >= 8 {
		if v, ok := cameraOUI[m[:8]]; ok {
			return v
		}
	}
	return ""
}

func randomUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func checkLinuxCarrier(ifname string) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	b, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/carrier", ifname))
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(b)) == "1"
}

func classifyInterface(ifname string) string {
	lname := strings.ToLower(ifname)
	if runtime.GOOS != "windows" {
		if _, err := os.Stat(fmt.Sprintf("/sys/class/net/%s/wireless", ifname)); err == nil {
			return "wifi"
		}
		if _, err := os.Stat(fmt.Sprintf("/sys/class/net/%s/phy80211", ifname)); err == nil {
			return "wifi"
		}
	}
	if strings.HasPrefix(lname, "wl") || strings.Contains(lname, "wi-fi") || strings.Contains(lname, "wlan") || strings.Contains(lname, "wireless") {
		return "wifi"
	}
	return "wired"
}

// Собирает информацию обо всех физических интерфейсах (Проводной кабель Ethernet + Wi-Fi),
// чтобы корректно работать, когда одновременно подключены и Wi-Fi (интернет), и кабель (камера/свитч).
func getAllNetworkInterfaces() []NetInterfaceInfo {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var list []NetInterfaceInfo
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		lname := strings.ToLower(iface.Name)
		if strings.Contains(lname, "tailscale") || strings.Contains(lname, "virtual") ||
			strings.Contains(lname, "docker") || strings.Contains(lname, "veth") ||
			strings.Contains(lname, "virbr") || strings.Contains(lname, "tun") ||
			strings.Contains(lname, "br-") || strings.Contains(lname, "wg") || strings.Contains(lname, "vmnet") {
			continue
		}

		if runtime.GOOS != "windows" && iface.Flags&net.FlagUp == 0 {
			_ = runPrivilegedLinux("ip", "link", "set", iface.Name, "up")
		}

		kind := classifyInterface(iface.Name)
		carrier := (iface.Flags&net.FlagUp != 0) && checkLinuxCarrier(iface.Name)
		var primaryIP string
		var primaryNet *net.IPNet
		var subnetStr string

		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				ipStr := ipnet.IP.String()
				if !strings.HasSuffix(ipStr, ".250") && !strings.HasSuffix(ipStr, ".249") && !strings.HasPrefix(ipStr, "169.254.") {
					primaryIP = ipStr
					primaryNet = ipnet
					parts := strings.Split(ipStr, ".")
					if len(parts) == 4 {
						subnetStr = fmt.Sprintf("%s.%s.%s.0/24", parts[0], parts[1], parts[2])
					}
					break
				}
			}
		}
		if primaryIP != "" {
			carrier = true
		}

		kindRu := "Кабель"
		if kind == "wifi" {
			kindRu = "Wi-Fi"
		}
		ipDisp := primaryIP
		if ipDisp == "" {
			if carrier {
				ipDisp = "Прямой линк (L2)"
			} else {
				ipDisp = "Отключен"
			}
		}
		list = append(list, NetInterfaceInfo{
			Name:    iface.Name,
			Kind:    kind,
			Label:   fmt.Sprintf("%s (%s · %s)", kindRu, iface.Name, ipDisp),
			IP:      primaryIP,
			Subnet:  subnetStr,
			Carrier: carrier,
			IPNet:   primaryNet,
		})
	}

	sort.Slice(list, func(i, j int) bool {
		if list[i].Carrier != list[j].Carrier {
			return list[i].Carrier
		}
		if list[i].Kind != list[j].Kind {
			return list[i].Kind == "wired"
		}
		return list[i].Name < list[j].Name
	})
	return list
}

func getPrimaryInterface() (string, string, *net.IPNet) {
	all := getAllNetworkInterfaces()
	// Для общего доступа и определения основной подсети роутера ищем интерфейс с реальным IPv4
	for _, inf := range all {
		if inf.IP != "" && inf.IPNet != nil {
			return inf.Name, inf.IP, inf.IPNet
		}
	}
	if len(all) > 0 {
		return all[0].Name, "192.168.1.250", nil
	}
	return "Ethernet", "192.168.1.250", nil
}

func selectInterfaceForCamera(preferredMode string) string {
	all := getAllNetworkInterfaces()
	if preferredMode != "" && preferredMode != "auto" {
		for _, inf := range all {
			if inf.Name == preferredMode || inf.Kind == preferredMode {
				return inf.Name
			}
		}
	}
	// Если подключен физический кабель (carrier=1), приоритет кабелю для L2/моста
	for _, inf := range all {
		if inf.Kind == "wired" && inf.Carrier {
			return inf.Name
		}
	}
	for _, inf := range all {
		if inf.Carrier {
			return inf.Name
		}
	}
	name, _, _ := getPrimaryInterface()
	return name
}

// Безопасная инъекция подсети при одновременной работе Wi-Fi и кабеля Ethernet:
// Если подсеть камеры совпадает с подсетью другого активного интерфейса (например, Wi-Fi),
// создает точечный маршрут /32 только до IP камеры, НЕ разрывая Wi-Fi и интернет!
func injectSubnetWindowsOrLinux(ifaceName, targetIP string) string {
	parsed := net.ParseIP(targetIP)
	if parsed == nil || parsed.To4() == nil {
		return "Некорректный IP"
	}
	allIfaces := getAllNetworkInterfaces()
	var targetIfaceInfo *NetInterfaceInfo
	var overlappingOther *NetInterfaceInfo

	for i := range allIfaces {
		inf := &allIfaces[i]
		if inf.Name == ifaceName {
			targetIfaceInfo = inf
		} else if inf.IPNet != nil && inf.IPNet.Contains(parsed) {
			overlappingOther = inf
		}
	}

	if targetIfaceInfo != nil && targetIfaceInfo.IPNet != nil && targetIfaceInfo.IPNet.Contains(parsed) && overlappingOther == nil {
		return fmt.Sprintf("Родная сеть (%s · %s)", targetIfaceInfo.Name, targetIfaceInfo.Subnet)
	}

	parts := strings.Split(targetIP, ".")
	if len(parts) != 4 {
		return "-"
	}
	aliasIP := fmt.Sprintf("%s.%s.%s.250", parts[0], parts[1], parts[2])
	if parts[3] == "250" || (overlappingOther != nil && overlappingOther.IP == aliasIP) {
		aliasIP = fmt.Sprintf("%s.%s.%s.249", parts[0], parts[1], parts[2])
	}

	stateMu.Lock()
	defer stateMu.Unlock()

	// Если Wi-Fi уже находится в той же /24 подсети, что и камера на кабеле Ethernet —
	// создаем точечный /32 маршрут на камеру через кабель, чтобы не перебить маршрут к роутеру по Wi-Fi!
	if overlappingOther != nil && runtime.GOOS != "windows" {
		_ = runPrivilegedLinux("ip", "addr", "add", fmt.Sprintf("%s/32", aliasIP), "dev", ifaceName, "label", fmt.Sprintf("%s:yeee", ifaceName))
		_ = runPrivilegedLinux("ip", "route", "replace", fmt.Sprintf("%s/32", targetIP), "dev", ifaceName, "src", aliasIP, "scope", "link")
		injectedSubnets[aliasIP+"/32"] = ifaceName
		injectedRoutes[targetIP+"/32"] = ifaceName
		return fmt.Sprintf("Точечный мост /32 (%s, Wi-Fi сохранен)", ifaceName)
	}

	if _, exists := injectedSubnets[aliasIP]; exists {
		return fmt.Sprintf("Мост %s/24 (%s)", aliasIP, ifaceName)
	}

	if runtime.GOOS == "windows" {
		_ = runHiddenCmd("netsh", "interface", "ipv4", "add", "address",
			fmt.Sprintf("name=%s", ifaceName),
			fmt.Sprintf("addr=%s", aliasIP),
			"mask=255.255.255.0")
	} else {
		_ = runPrivilegedLinux("ip", "addr", "add", fmt.Sprintf("%s/24", aliasIP), "brd", "+", "dev", ifaceName, "label", fmt.Sprintf("%s:yeee", ifaceName))
	}
	injectedSubnets[aliasIP] = ifaceName
	return fmt.Sprintf("Мост %s/24 (%s)", aliasIP, ifaceName)
}

func cleanSubnetsWindowsOrLinux() string {
	if runtime.GOOS != "windows" {
		if _, err := os.Stat("/usr/local/bin/yeee"); err == nil {
			_ = runPrivilegedLinux("/usr/local/bin/yeee", "clean")
		}
	}
	stateMu.Lock()
	defer stateMu.Unlock()
	count := 0
	for routeDst, ifaceName := range injectedRoutes {
		if runtime.GOOS != "windows" {
			_ = runPrivilegedLinux("ip", "route", "del", routeDst, "dev", ifaceName)
		}
		delete(injectedRoutes, routeDst)
	}
	for aliasKey, ifaceName := range injectedSubnets {
		if runtime.GOOS == "windows" {
			cleanIP := strings.Split(aliasKey, "/")[0]
			_ = runHiddenCmd("netsh", "interface", "ipv4", "delete", "address",
				fmt.Sprintf("name=%s", ifaceName),
				fmt.Sprintf("addr=%s", cleanIP))
		} else {
			cidr := aliasKey
			if !strings.Contains(cidr, "/") {
				cidr += "/24"
			}
			_ = runPrivilegedLinux("ip", "addr", "del", cidr, "dev", ifaceName)
		}
		delete(injectedSubnets, aliasKey)
		count++
	}
	return fmt.Sprintf("Временные сети и мосты очищены (%d). Основное соединение Wi-Fi / роутера работает штатно.", count)
}

func pinStaticARP(ifaceName, ipStr, macStr string) {
	if macStr == "" {
		return
	}
	if runtime.GOOS == "windows" {
		winMac := strings.ToLower(strings.ReplaceAll(macStr, ":", "-"))
		_ = runHiddenCmd("netsh", "interface", "ipv4", "set", "neighbors", ifaceName, ipStr, winMac)
		_ = runHiddenCmd("arp", "-s", ipStr, winMac)
	} else {
		colonMac := strings.ToLower(strings.ReplaceAll(macStr, "-", ":"))
		_ = runPrivilegedLinux("ip", "neigh", "replace", ipStr, "lladdr", colonMac, "nud", "permanent", "dev", ifaceName)
	}
}

func unpinStaticARP(ifaceName, ipStr string) {
	if runtime.GOOS == "windows" {
		_ = runHiddenCmd("arp", "-d", ipStr)
	} else {
		_ = runPrivilegedLinux("ip", "neigh", "del", ipStr, "dev", ifaceName)
	}
}

func parseDiscoveryPayload(raw []byte) (ip, mac, vendor, model, serial, fw string) {
	txt := string(raw)
	reIP := regexp.MustCompile(`(?i)<IPv4Address>\s*(\d+\.\d+\.\d+\.\d+)\s*</IPv4Address>`)
	if m := reIP.FindStringSubmatch(txt); len(m) > 1 {
		ip = m[1]
		vendor = "Hikvision / HiWatch"
	}
	reMAC := regexp.MustCompile(`(?i)<MAC>\s*([0-9a-fA-F:-]{17})\s*</MAC>`)
	if m := reMAC.FindStringSubmatch(txt); len(m) > 1 {
		mac = strings.ToUpper(strings.ReplaceAll(m[1], "-", ":"))
	}
	reDesc := regexp.MustCompile(`(?i)<(?:DeviceDescription|DeviceType)>\s*([^<]+)\s*</(?:DeviceDescription|DeviceType)>`)
	if m := reDesc.FindStringSubmatch(txt); len(m) > 1 {
		model = strings.TrimSpace(m[1])
	}
	reSN := regexp.MustCompile(`(?i)<(?:DeviceSN|SerialNumber)>\s*([^<]+)\s*</(?:DeviceSN|SerialNumber)>`)
	if m := reSN.FindStringSubmatch(txt); len(m) > 1 {
		serial = strings.TrimSpace(m[1])
	}
	reFW := regexp.MustCompile(`(?i)<SoftwareVersion>\s*([^<]+)\s*</SoftwareVersion>`)
	if m := reFW.FindStringSubmatch(txt); len(m) > 1 {
		fw = strings.TrimSpace(m[1])
	}

	if strings.Contains(txt, "ProbeMatches") || strings.Contains(txt, "XAddrs") {
		reOnvifIP := regexp.MustCompile(`https?://(\d+\.\d+\.\d+\.\d+)`)
		if m := reOnvifIP.FindStringSubmatch(txt); len(m) > 1 && ip == "" {
			ip = m[1]
		}
		upTxt := strings.ToUpper(txt)
		if model == "" && (strings.Contains(upTxt, "NETWORKVIDEOSTORAGE") || strings.Contains(upTxt, "NVR") || strings.Contains(upTxt, "DVR") || strings.Contains(upTxt, "XVR") || strings.Contains(upTxt, "RECORDER")) {
			model = "ONVIF NVR"
		}
		if vendor == "" {
			vendor = "ONVIF Device"
		}
	}
	if strings.Contains(txt, "DHDiscover") || strings.Contains(txt, `"DeviceType"`) {
		reDhIP := regexp.MustCompile(`"IPAddress"\s*:\s*"(\d+\.\d+\.\d+\.\d+)"`)
		if m := reDhIP.FindStringSubmatch(txt); len(m) > 1 && ip == "" {
			ip = m[1]
		}
		reDhMAC := regexp.MustCompile(`(?i)"mac"\s*:\s*"([0-9a-fA-F:-]{17})"`)
		if m := reDhMAC.FindStringSubmatch(txt); len(m) > 1 && mac == "" {
			mac = strings.ToUpper(strings.ReplaceAll(m[1], "-", ":"))
		}
		reDhModel := regexp.MustCompile(`"DeviceType"\s*:\s*"([^"]+)"`)
		if m := reDhModel.FindStringSubmatch(txt); len(m) > 1 && model == "" {
			model = m[1]
		}
		vendor = "Dahua"
	}
	return
}

type ARPRecord struct {
	MAC   string
	Iface string
}

func readSystemARPTable() map[string][]ARPRecord {
	res := make(map[string][]ARPRecord)
	addEntry := func(ip, mac, dev string) {
		mac = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(mac), "-", ":"))
		if mac == "FF:FF:FF:FF:FF:FF" || mac == "00:00:00:00:00:00" || len(mac) != 17 {
			return
		}
		for _, ex := range res[ip] {
			if ex.MAC == mac {
				return
			}
		}
		res[ip] = append(res[ip], ARPRecord{MAC: mac, Iface: dev})
	}

	if runtime.GOOS != "windows" {
		if data, err := os.ReadFile("/proc/net/arp"); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				fields := strings.Fields(line)
				if len(fields) >= 6 && net.ParseIP(fields[0]) != nil && fields[2] != "0x0" {
					addEntry(fields[0], fields[3], fields[5])
				}
			}
		}
	}

	out, err := runHiddenOutput("arp", "-a")
	if err == nil {
		reLine := regexp.MustCompile(`(\d+\.\d+\.\d+\.\d+)\s+.*?([0-9a-fA-F]{2}[:-][0-9a-fA-F]{2}[:-][0-9a-fA-F]{2}[:-][0-9a-fA-F]{2}[:-][0-9a-fA-F]{2}[:-][0-9a-fA-F]{2})`)
		for _, line := range strings.Split(string(out), "\n") {
			if m := reLine.FindStringSubmatch(line); len(m) > 2 {
				addEntry(m[1], m[2], "")
			}
		}
	}
	return res
}

func scanPorts(ip string) []int {
	targetPorts := []int{80, 443, 554, 8000, 37777, 34567}
	var open []int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range targetPorts {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), 450*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				mu.Lock()
				open = append(open, port)
				mu.Unlock()
			}
		}(p)
	}
	wg.Wait()
	sort.Ints(open)
	return open
}

// Определение типа устройства: NVR / DVR / XVR (Видеорегистратор) или IP-Камера
func classifyDeviceRole(ip, vendor, model string, ports []int) (devType, devLabel, enrichedModel string) {
	enrichedModel = model
	upModel := strings.ToUpper(model)
	upVendor := strings.ToUpper(vendor)

	nvrKeywords := []string{
		"NVR", "DVR", "XVR", "HCVR", "CVR", "EVR",
		"DS-71", "DS-72", "DS-73", "DS-76", "DS-77", "DS-86", "DS-96", "DS-E0",
		"IDS-71", "IDS-72", "IDS-76", "IDS-77", "IDS-96",
		"DS-N2", "DS-N3", "DS-H1", "DS-H2", "MH-",
		"DHI-NVR", "DHI-XVR", "DHI-HCVR",
		"NVR1", "NVR2", "NVR4", "NVR5", "NVR6",
		"XVR1", "XVR4", "XVR5", "XVR7",
		"NVR30", "NVR50", "XVR30", "NETWORKVIDEOSTORAGE", "RECORDER",
	}
	for _, kw := range nvrKeywords {
		if strings.Contains(upModel, kw) || strings.Contains(upVendor, kw) {
			return "nvr", "NVR / Регистратор", enrichedModel
		}
	}

	// Быстрый HTTP-опрос заголовка Server и XML-паспорта (DNVRS-Webs = Hikvision NVR/DVR)
	hasHTTP := false
	has34567 := false
	for _, p := range ports {
		if p == 80 || p == 443 {
			hasHTTP = true
		}
		if p == 34567 {
			has34567 = true
		}
	}

	if hasHTTP {
		client := &http.Client{
			Timeout: 350 * time.Millisecond,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
		for _, path := range []string{"/ISAPI/System/deviceInfo", "/doc/page/login.asp", "/"} {
			resp, err := client.Get(fmt.Sprintf("http://%s%s", ip, path))
			if err != nil {
				continue
			}
			srv := strings.ToUpper(resp.Header.Get("Server"))
			bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			bodyStr := string(bodyBytes)
			upBody := strings.ToUpper(bodyStr)

			if enrichedModel == "" {
				reMod := regexp.MustCompile(`(?i)<model>([^<]+)</model>`)
				if m := reMod.FindStringSubmatch(bodyStr); len(m) > 1 {
					enrichedModel = strings.TrimSpace(m[1])
					upModel = strings.ToUpper(enrichedModel)
				}
			}

			// Hikvision / HiWatch NVR всегда отдает заголовок Server: DNVRS-Webs
			if strings.Contains(srv, "DNVRS-WEBS") || strings.Contains(srv, "NVR") || strings.Contains(srv, "DVR") {
				return "nvr", "NVR / Регистратор", enrichedModel
			}
			if strings.Contains(upBody, "<DEVICETYPE>NVR") ||
				strings.Contains(upBody, "<DEVICETYPE>DVR") ||
				strings.Contains(upBody, "<DEVICETYPE>HYBRID") ||
				strings.Contains(upBody, "NETWORK VIDEO RECORDER") ||
				strings.Contains(upBody, "DIGITAL VIDEO RECORDER") {
				return "nvr", "NVR / Регистратор", enrichedModel
			}
			for _, kw := range nvrKeywords {
				if strings.Contains(upModel, kw) {
					return "nvr", "NVR / Регистратор", enrichedModel
				}
			}
			break
		}
	}

	if has34567 && (strings.Contains(upVendor, "XIONGMAI") || strings.Contains(upModel, "DVR") || strings.Contains(upModel, "NVR")) {
		return "nvr", "NVR / Регистратор", enrichedModel
	}

	return "camera", "IP-Камера", enrichedModel
}

// Двухуровневое сканирование сразу по всем выбранным интерфейсам (Проводной кабель + Wi-Fi)
func runFullRadarScan(ifaceFilter string) []CameraEntry {
	allIfaces := getAllNetworkInterfaces()
	var activeIfaces []NetInterfaceInfo
	myIPs := make(map[string]bool)
	for _, inf := range allIfaces {
		if inf.IP != "" {
			myIPs[inf.IP] = true
		}
		if !inf.Carrier {
			continue
		}
		if ifaceFilter == "" || ifaceFilter == "auto" || inf.Name == ifaceFilter || inf.Kind == ifaceFilter {
			activeIfaces = append(activeIfaces, inf)
		}
	}
	if len(activeIfaces) == 0 && len(allIfaces) > 0 {
		activeIfaces = append(activeIfaces, allIfaces[0])
	}

	// На Linux дополнительно вызываем L2 Scapy-радар (/usr/local/bin/yeee scan), если он установлен,
	// чтобы перехватить сырые L2 пакеты от камер без IP/в чужих подсетях на кабеле без DHCP.
	if runtime.GOOS != "windows" {
		if _, err := os.Stat("/usr/local/bin/yeee"); err == nil {
			_ = runPrivilegedLinux("/usr/local/bin/yeee", "scan")
		}
	}

	found := make(map[string]*CameraEntry)
	var mu sync.Mutex

	// Подгружаем данные из кэша Scapy L2 (/var/tmp/yeee_last_scan.json), если они есть
	if runtime.GOOS != "windows" {
		if raw, err := os.ReadFile("/var/tmp/yeee_last_scan.json"); err == nil {
			var cacheObj struct {
				Cameras []struct {
					IP            string   `json:"ip"`
					MAC           string   `json:"mac"`
					Vendor        string   `json:"vendor_resolved"`
					Model         string   `json:"model"`
					Serial        string   `json:"serial"`
					Firmware      string   `json:"firmware"`
					OpenPorts     []int    `json:"open_ports"`
					Iface         string   `json:"iface"`
					IsLocalSubnet bool     `json:"is_local_subnet"`
					Conflict      bool     `json:"conflict"`
				} `json:"cameras"`
			}
			if json.Unmarshal(raw, &cacheObj) == nil {
				for _, c := range cacheObj.Cameras {
					if c.IP == "" || c.IP == "0.0.0.0" || myIPs[c.IP] {
						continue
					}
					if ifaceFilter != "" && ifaceFilter != "auto" && c.Iface != "" && c.Iface != ifaceFilter {
						kind := classifyInterface(c.Iface)
						if kind != ifaceFilter {
							continue
						}
					}
					macUp := strings.ToUpper(c.MAC)
					key := c.IP + "|" + macUp
					found[key] = &CameraEntry{
						IP:       c.IP,
						MAC:      macUp,
						Vendor:   c.Vendor,
						Model:    c.Model,
						Serial:   c.Serial,
						Firmware: c.Firmware,
						Ports:    c.OpenPorts,
						Iface:    c.Iface,
						Conflict: c.Conflict,
					}
				}
			}
		}
	}

	var wg sync.WaitGroup
	// Отправляем UDP-пробы ONVIF / SADP / Dahua
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := net.ListenPacket("udp4", ":0")
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(2200 * time.Millisecond))

		onvifXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2003/05/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl"><e:Header><w:MessageID>uuid:%s</w:MessageID><w:To>urn:schemas-xmlsoap-org:ws:2005:04/discovery</w:To><w:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action></e:Header><e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter</d:Types></d:Probe></e:Body></e:Envelope>`, randomUUID())
		onvifNvrXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl"><e:Header><w:MessageID>uuid:%s</w:MessageID><w:To>urn:schemas-xmlsoap-org:ws:2005:04/discovery</w:To><w:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action></e:Header><e:Body><d:Probe><d:Types>dn:NetworkVideoStorage</d:Types></d:Probe></e:Body></e:Envelope>`, randomUUID())
		sadpXML := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>%s</Uuid><Types>inquiry</Types></Probe>`, strings.ToUpper(randomUUID()))
		dahuaJSON := `{"method":"DHDiscover.search","params":{"mac":"","uni":1}}`

		targets := []struct {
			addr string
			data []byte
		}{
			{"239.255.255.250:3702", []byte(onvifXML)},
			{"255.255.255.255:3702", []byte(onvifXML)},
			{"239.255.255.250:3702", []byte(onvifNvrXML)},
			{"255.255.255.255:3702", []byte(onvifNvrXML)},
			{"239.255.255.250:37020", []byte(sadpXML)},
			{"255.255.255.255:37020", []byte(sadpXML)},
			{"239.255.255.251:37810", []byte(dahuaJSON)},
			{"255.255.255.255:37810", []byte(dahuaJSON)},
		}
		for _, t := range targets {
			if raddr, err := net.ResolveUDPAddr("udp4", t.addr); err == nil {
				_, _ = conn.WriteTo(t.data, raddr)
			}
		}

		buf := make([]byte, 8192)
		for {
			n, raddr, err := conn.ReadFrom(buf)
			if err != nil {
				break
			}
			ip, mac, vendor, model, serial, fw := parseDiscoveryPayload(buf[:n])
			if ip == "" && raddr != nil {
				if u, ok := raddr.(*net.UDPAddr); ok {
					ip = u.IP.String()
				}
			}
			if ip == "" || myIPs[ip] || ip == "0.0.0.0" {
				continue
			}
			mu.Lock()
			key := ip + "|" + mac
			if _, exists := found[key]; !exists {
				found[key] = &CameraEntry{
					IP:       ip,
					MAC:      mac,
					Vendor:   vendor,
					Model:    model,
					Serial:   serial,
					Firmware: fw,
				}
			}
			mu.Unlock()
		}
	}()

	// Опрашиваем родные /24 подсети всех выбранных интерфейсов
	for _, inf := range activeIfaces {
		if inf.IP == "" {
			continue
		}
		parts := strings.Split(inf.IP, ".")
		if len(parts) == 4 {
			prefix := fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
			var sweepWG sync.WaitGroup
			sem := make(chan struct{}, 64)
			for i := 1; i <= 254; i++ {
				candIP := fmt.Sprintf("%s.%d", prefix, i)
				if myIPs[candIP] {
					continue
				}
				sweepWG.Add(1)
				go func(target string) {
					defer sweepWG.Done()
					sem <- struct{}{}
					defer func() { <-sem }()
					for _, p := range []int{554, 8000, 37777, 80} {
						c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", target, p), 140*time.Millisecond)
						if err == nil {
							_ = c.Close()
							break
						}
					}
				}(candIP)
			}
			sweepWG.Wait()
		}
	}

	wg.Wait()
	arpMap := readSystemARPTable()

	for ip, recs := range arpMap {
		if myIPs[ip] {
			continue
		}
		for _, rec := range recs {
			if ifaceFilter != "" && ifaceFilter != "auto" && rec.Iface != "" && rec.Iface != ifaceFilter {
				if classifyInterface(rec.Iface) != ifaceFilter {
					continue
				}
			}
			v := lookupVendor(rec.MAC)
			ports := scanPorts(ip)
			hasCamPort := false
			for _, p := range ports {
				if p == 554 || p == 8000 || p == 37777 || p == 34567 {
					hasCamPort = true
				}
			}
			if v != "" || hasCamPort || len(recs) > 1 {
				mu.Lock()
				key := ip + "|" + rec.MAC
				if ex, exists := found[key]; !exists {
					found[key] = &CameraEntry{
						IP:       ip,
						MAC:      rec.MAC,
						Vendor:   v,
						Ports:    ports,
						Iface:    rec.Iface,
						Conflict: len(recs) > 1,
					}
				} else {
					if len(ex.Ports) == 0 {
						ex.Ports = ports
					}
					if ex.Iface == "" {
						ex.Iface = rec.Iface
					}
					if len(recs) > 1 {
						ex.Conflict = true
					}
				}
				mu.Unlock()
			}
		}
	}

	var result []CameraEntry
	for _, entry := range found {
		if entry.MAC == "" {
			if mlist, ok := arpMap[entry.IP]; ok && len(mlist) > 0 {
				entry.MAC = mlist[0].MAC
				if entry.Iface == "" {
					entry.Iface = mlist[0].Iface
				}
			}
		}
		if entry.Iface == "" {
			entry.Iface = selectInterfaceForCamera(ifaceFilter)
		}
		entry.IfaceKind = classifyInterface(entry.Iface)
		if entry.Vendor == "" {
			entry.Vendor = lookupVendor(entry.MAC)
			if entry.Vendor == "" {
				entry.Vendor = "IP-камера (ONVIF)"
			}
		}
		entry.SubnetAlias = injectSubnetWindowsOrLinux(entry.Iface, entry.IP)
		if len(entry.Ports) == 0 {
			entry.Ports = scanPorts(entry.IP)
		}
		entry.DeviceType, entry.DeviceLabel, entry.Model = classifyDeviceRole(entry.IP, entry.Vendor, entry.Model, entry.Ports)

		isLocal := false
		parsedIP := net.ParseIP(entry.IP)
		for _, inf := range allIfaces {
			if inf.IPNet != nil && parsedIP != nil && inf.IPNet.Contains(parsedIP) {
				isLocal = true
				break
			}
		}

		if entry.Conflict {
			entry.Zone = "Конфликт IP"
			entry.ZoneType = "conflict"
		} else if isLocal {
			entry.Zone = "Своя сеть"
			entry.ZoneType = "local"
		} else {
			entry.Zone = "Чужая сеть"
			entry.ZoneType = "foreign"
		}
		entry.WebLink = fmt.Sprintf("http://%s", entry.IP)
		entry.RTSPLink = fmt.Sprintf("rtsp://admin:12345@%s:554/Streaming/Channels/101", entry.IP)
		result = append(result, *entry)
	}

	sort.Slice(result, func(i, j int) bool { return result[i].IP < result[j].IP })
	stateMu.Lock()
	cachedCameras = result
	stateMu.Unlock()
	return result
}

// Интерактивный режим «Поиск по кабелю (Отключи -> Включи)»
func runPlugCaptureScan(ifaceFilter string) []CameraEntry {
	targetIface := selectInterfaceForCamera(ifaceFilter)
	_, myIP, _ := getPrimaryInterface()

	baselineOnline := make(map[string]bool)
	arp0 := readSystemARPTable()
	for ip := range arp0 {
		if ip != myIP {
			baselineOnline[ip] = true
		}
	}

	unpluggedSet := make(map[string]bool)
	var captured []CameraEntry

	deadline := time.Now().Add(24 * time.Second)
	round := 0
	for time.Now().Before(deadline) {
		round++
		for ip := range baselineOnline {
			c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:80", ip), 150*time.Millisecond)
			if err != nil {
				c2, err2 := net.DialTimeout("tcp", fmt.Sprintf("%s:554", ip), 150*time.Millisecond)
				if err2 != nil {
					unpluggedSet[ip] = true
				} else {
					_ = c2.Close()
				}
			} else {
				_ = c.Close()
			}
		}

		factoryIPs := []string{"192.168.1.64", "192.168.1.108", "192.168.1.10", "192.168.0.64", "192.168.0.108"}
		for _, fip := range factoryIPs {
			injectSubnetWindowsOrLinux(targetIface, fip)
		}

		cams := runFullRadarScan(ifaceFilter)
		if round >= 2 {
			var newlyPlugged []CameraEntry
			for i := range cams {
				cam := &cams[i]
				if !baselineOnline[cam.IP] || unpluggedSet[cam.IP] || cam.ZoneType == "foreign" {
					cam.Zone = "Поймана по кабелю"
					newlyPlugged = append(newlyPlugged, *cam)
				}
			}
			if len(newlyPlugged) > 0 {
				captured = cams
				break
			}
		}
		captured = cams
		time.Sleep(1500 * time.Millisecond)
	}
	return captured
}

type FreePoolResponse struct {
	Subnet          string   `json:"subnet"`
	Gateway         string   `json:"gateway"`
	RequestedCount  int      `json:"requested_count"`
	TotalFreeInSub  int      `json:"total_free_in_subnet"`
	OccupiedInSub   int      `json:"occupied_in_subnet"`
	RangeStart      string   `json:"range_start"`
	RangeEnd        string   `json:"range_end"`
	RecommendedList []string `json:"recommended_list"`
}

// Подбор свободных IP с выбором нужного количества (count) и целевой подсети (targetSubnet)
func findFreeSubnetIPs(count int, targetSubnet string) FreePoolResponse {
	if count < 1 {
		count = 1
	}
	if count > 245 {
		count = 245
	}

	_, myIP, _ := getPrimaryInterface()
	sourceSpec := targetSubnet
	if sourceSpec == "" {
		sourceSpec = myIP
	}
	cleanIP := strings.Split(strings.TrimSpace(sourceSpec), "/")[0]
	parts := strings.Split(cleanIP, ".")
	if len(parts) < 3 {
		parts = []string{"192", "168", "1", "0"}
	}
	prefix := fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
	arpMap := readSystemARPTable()

	busyMap := make(map[int]bool)
	busyMap[1] = true
	busyMap[248] = true
	busyMap[249] = true
	busyMap[250] = true
	if myParts := strings.Split(myIP, "."); len(myParts) == 4 && fmt.Sprintf("%s.%s.%s", myParts[0], myParts[1], myParts[2]) == prefix {
		if mHost, err := strconv.Atoi(myParts[3]); err == nil {
			busyMap[mHost] = true
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 48)

	for host := 2; host <= 247; host++ {
		if busyMap[host] {
			continue
		}
		cand := fmt.Sprintf("%s.%d", prefix, host)
		if len(arpMap[cand]) > 0 {
			busyMap[host] = true
			continue
		}
		wg.Add(1)
		go func(h int, ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			for _, p := range []int{80, 554, 8000, 37777} {
				if c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, p), 110*time.Millisecond); err == nil {
					_ = c.Close()
					mu.Lock()
					busyMap[h] = true
					mu.Unlock()
					break
				}
			}
		}(host, cand)
	}
	wg.Wait()

	var allFree []int
	var preferredFree []int
	for h := 2; h <= 247; h++ {
		if !busyMap[h] {
			allFree = append(allFree, h)
			if h >= 100 && h <= 244 {
				preferredFree = append(preferredFree, h)
			}
		}
	}

	pool := preferredFree
	if len(pool) < count {
		pool = allFree
	}

	// Ищем непрерывный блок из count адресов
	var bestBlock []int
	var curBlock []int
	for idx, h := range pool {
		if idx == 0 || h == pool[idx-1]+1 {
			curBlock = append(curBlock, h)
		} else {
			curBlock = []int{h}
		}
		if len(curBlock) == count {
			bestBlock = append([]int(nil), curBlock...)
			break
		}
	}
	if len(bestBlock) == 0 {
		if len(pool) >= count {
			bestBlock = pool[:count]
		} else {
			bestBlock = pool
		}
	}

	var recIPs []string
	for _, h := range bestBlock {
		recIPs = append(recIPs, fmt.Sprintf("%s.%d", prefix, h))
	}
	rangeStart, rangeEnd := "-", "-"
	if len(recIPs) > 0 {
		rangeStart = recIPs[0]
		rangeEnd = recIPs[len(recIPs)-1]
	}

	return FreePoolResponse{
		Subnet:          fmt.Sprintf("%s.0/24", prefix),
		Gateway:         fmt.Sprintf("%s.1", prefix),
		RequestedCount:  count,
		TotalFreeInSub:  len(allFree),
		OccupiedInSub:   246 - len(allFree),
		RangeStart:      rangeStart,
		RangeEnd:        rangeEnd,
		RecommendedList: recIPs,
	}
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func doDigestRequest(method, urlStr, user, pass string, body []byte) (int, string, error) {
	client := &http.Client{
		Timeout:   3 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	req1, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		return 0, "", err
	}
	resp1, err := client.Do(req1)
	if err != nil {
		return 0, "", err
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp1.Body)
		return resp1.StatusCode, string(b), nil
	}

	authHeader := resp1.Header.Get("WWW-Authenticate")
	extract := func(key string) string {
		re := regexp.MustCompile(key + `="([^"]+)"`)
		if m := re.FindStringSubmatch(authHeader); len(m) > 1 {
			return m[1]
		}
		return ""
	}
	realm := extract("realm")
	nonce := extract("nonce")
	qop := extract("qop")
	uri := req1.URL.RequestURI()
	cnonce := "0a4f113b"
	nc := "00000001"

	ha1 := md5Hex(fmt.Sprintf("%s:%s:%s", user, realm, pass))
	ha2 := md5Hex(fmt.Sprintf("%s:%s", method, uri))
	var responseHash string
	if strings.Contains(qop, "auth") {
		responseHash = md5Hex(fmt.Sprintf("%s:%s:%s:%s:auth:%s", ha1, nonce, nc, cnonce, ha2))
	} else {
		responseHash = md5Hex(fmt.Sprintf("%s:%s:%s", ha1, nonce, ha2))
	}

	digestVal := fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="%s", response="%s"`,
		user, realm, nonce, uri, responseHash)
	if strings.Contains(qop, "auth") {
		digestVal += fmt.Sprintf(`, qop=auth, nc=%s, cnonce="%s"`, nc, cnonce)
	}

	req2, err := http.NewRequest(method, urlStr, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req2.Header.Set("Authorization", digestVal)
	if len(body) > 0 {
		req2.Header.Set("Content-Type", "application/xml")
	}
	resp2, err := client.Do(req2)
	if err != nil {
		return 0, "", err
	}
	defer resp2.Body.Close()
	b2, _ := io.ReadAll(resp2.Body)
	return resp2.StatusCode, string(b2), nil
}

func openExternalTarget(target string) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
		prepareBackgroundCmd(cmd)
		_ = cmd.Start()
	} else {
		if os.Geteuid() == 0 && os.Getenv("SUDO_USER") != "" {
			_ = exec.Command("sudo", "-u", os.Getenv("SUDO_USER"), "xdg-open", target).Start()
		} else {
			_ = exec.Command("xdg-open", target).Start()
		}
	}
}

func openRTSPPlayer(rtspURL string) string {
	players := []struct {
		bin  string
		args []string
	}{
		{"mpv", []string{"--rtsp-transport=tcp", "--title=ProCam Radar RTSP", rtspURL}},
		{"vlc", []string{rtspURL}},
		{"ffplay", []string{"-rtsp_transport", "tcp", "-window_title", "ProCam Radar RTSP", rtspURL}},
	}
	for _, p := range players {
		if path, err := exec.LookPath(p.bin); err == nil {
			cmd := exec.Command(path, p.args...)
			prepareBackgroundCmd(cmd)
			_ = cmd.Start()
			return fmt.Sprintf("Запущен видеоплеер %s -> %s", p.bin, rtspURL)
		}
	}
	openExternalTarget(rtspURL)
	return fmt.Sprintf("Открыт видеопоток -> %s", rtspURL)
}

func launchStandaloneDesktopApp(url string) {
	tmpDir, err := os.MkdirTemp("", "procam-radar-window-*")
	if err == nil {
		appTempDir = tmpDir
		_ = os.WriteFile(filepath.Join(tmpDir, "First Run"), []byte(""), 0644)
	}

	commonFlags := []string{
		"--app=" + url,
		"--window-size=1340,860",
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-extensions",
		"--disable-sync",
		"--disable-translate",
		"--disable-features=Translate,OverscrollHistoryNavigation",
	}
	if appTempDir != "" {
		commonFlags = append(commonFlags, "--user-data-dir="+appTempDir)
	}

	if runtime.GOOS == "windows" {
		var candidates []string
		for _, envKey := range []string{"ProgramFiles(x86)", "ProgramFiles", "LocalAppData"} {
			base := os.Getenv(envKey)
			if base != "" {
				candidates = append(candidates,
					filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
					filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
					filepath.Join(base, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
				)
			}
		}
		for _, bin := range candidates {
			if _, err := os.Stat(bin); err == nil {
				cmd := exec.Command(bin, commonFlags...)
				if err := cmd.Start(); err == nil {
					go func() {
						_ = cmd.Wait()
						cleanupAndExit()
					}()
					return
				}
			}
		}
		cmd := exec.Command("cmd", "/c", "start", "", "msedge", "--app="+url, "--window-size=1340,860")
		prepareBackgroundCmd(cmd)
		if err := cmd.Start(); err == nil {
			return
		}
		openExternalTarget(url)
		return
	}

	linuxEngines := []string{
		"google-chrome",
		"google-chrome-stable",
		"chromium",
		"chromium-browser",
		"brave-browser",
		"microsoft-edge",
	}
	for _, name := range linuxEngines {
		if binPath, err := exec.LookPath(name); err == nil {
			args := append([]string{"--ozone-platform=x11", "--class=ProCamRadar"}, commonFlags...)
			var cmd *exec.Cmd
			if os.Geteuid() == 0 {
				if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
					if appTempDir != "" {
						_ = exec.Command("chmod", "-R", "777", appTempDir).Run()
					}
					sudoArgs := []string{
						"-u", sudoUser,
						"--preserve-env=DISPLAY,WAYLAND_DISPLAY,XAUTHORITY,XDG_RUNTIME_DIR,DBUS_SESSION_BUS_ADDRESS",
						binPath,
					}
					sudoArgs = append(sudoArgs, args...)
					cmd = exec.Command("sudo", sudoArgs...)
				} else {
					args = append(args, "--no-sandbox")
					cmd = exec.Command(binPath, args...)
				}
			} else {
				cmd = exec.Command(binPath, args...)
			}
			if err := cmd.Start(); err == nil {
				go func() {
					_ = cmd.Wait()
					cleanupAndExit()
				}()
				return
			}
		}
	}

	openExternalTarget(url)
}

func cleanupAndExit() {
	cleanSubnetsWindowsOrLinux()
	if appTempDir != "" {
		_ = os.RemoveAll(appTempDir)
	}
	os.Exit(0)
}

//go:embed icon.png
var embeddedIconPNG []byte

const appIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100" fill="none">
  <rect width="100" height="100" rx="22" fill="#09090B"/>
  <path d="M50 8 L88 26 L88 54 C88 76 50 92 50 92 C50 92 12 76 12 54 L12 26 Z" fill="#18181B" stroke="#F59E0B" stroke-width="4" stroke-linejoin="round"/>
  <polygon points="50,15 32,33 50,27 68,33" fill="#0284C7" opacity="0.88"/>
  <polygon points="18,22 36,33 23,46" fill="#F59E0B"/>
  <polygon points="82,22 64,33 77,46" fill="#F59E0B"/>
  <polygon points="21,45 45,45 48,60 25,60" fill="#09090B" stroke="#38BDF8" stroke-width="2.5"/>
  <polygon points="79,45 55,45 52,60 75,60" fill="#09090B" stroke="#38BDF8" stroke-width="2.5"/>
  <circle cx="35" cy="52.5" r="5" fill="#F59E0B"/>
  <circle cx="65" cy="52.5" r="5" fill="#F59E0B"/>
  <polygon points="50,53 43,66 50,79 57,66" fill="#F59E0B"/>
</svg>`

const uiHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>ProCam Radar — Инженерная система поиска и настройки IP-камер</title>
<link rel="icon" type="image/png" sizes="256x256" href="/icon.png">
<link rel="icon" type="image/svg+xml" href="/favicon.svg">
<link rel="shortcut icon" href="/icon.png">
<link rel="apple-touch-icon" href="/icon.png">
<style>
  :root {
    --bg: #FAFAFA;
    --surface: #FFFFFF;
    --surface-subtle: #F4F4F5;
    --dark: #09090B;
    --dark-card: #18181B;
    --dark-border: #27272A;
    --border: #E4E4E7;
    --text: #09090B;
    --muted: #52525B;
    --muted-light: #71717A;
    --emerald: #10B981;
    --emerald-dark: #059669;
    --emerald-soft: rgba(16, 185, 129, 0.12);
    --amber: #D97706;
    --amber-soft: rgba(217, 119, 6, 0.12);
    --red: #EF4444;
    --red-soft: rgba(239, 68, 68, 0.12);
    --font-sans: "Inter Variable", "Inter", -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    --font-mono: ui-monospace, "SFMono-Regular", Menlo, Monaco, Consolas, "DejaVu Sans Mono", monospace;
  }

  * {
    box-sizing: border-box;
    margin: 0;
    padding: 0;
    font-family: var(--font-sans);
    -webkit-font-smoothing: antialiased;
  }

  body {
    background-color: var(--bg);
    background-image: radial-gradient(#D4D4D8 1px, transparent 1px);
    background-size: 24px 24px;
    color: var(--text);
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }

  /* ProCam Sticky Glass Header */
  .header {
    position: sticky;
    top: 0;
    z-index: 100;
    background: rgba(255, 255, 255, 0.92);
    backdrop-filter: blur(14px);
    -webkit-backdrop-filter: blur(14px);
    border-bottom: 1px solid var(--border);
    padding: 12px 24px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 14px;
  }

  .brand-group {
    display: flex;
    align-items: center;
    gap: 14px;
    flex-wrap: wrap;
  }

  .logo-box {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    text-decoration: none;
  }

  .wordmark-main {
    font-size: 19px;
    font-weight: 900;
    letter-spacing: -0.03em;
    color: var(--dark);
    line-height: 1.05;
  }
  .wordmark-main span.cam {
    font-weight: 400;
    color: var(--muted);
  }
  .wordmark-main span.radar-pill {
    display: inline-block;
    margin-left: 6px;
    padding: 2px 8px;
    font-size: 11px;
    font-weight: 800;
    font-family: var(--font-mono);
    background: var(--dark);
    color: var(--emerald);
    border-radius: 999px;
    vertical-align: middle;
    letter-spacing: 0.04em;
  }
  .wordmark-sub {
    font-size: 9.5px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--muted-light);
    margin-top: 2px;
  }

  /* Dual Network Status Pills (Ethernet + Wi-Fi) */
  .net-bar {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .net-pill {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 11.5px;
    font-family: var(--font-mono);
    font-weight: 600;
    background: var(--surface);
    border: 1px solid var(--border);
    padding: 5px 11px;
    border-radius: 999px;
    color: var(--text);
  }
  .net-pill .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #A1A1AA;
  }
  .net-pill.online .dot {
    background: var(--emerald);
    box-shadow: 0 0 0 3px var(--emerald-soft);
  }

  .iface-select {
    background: var(--dark);
    color: #FFFFFF;
    border: 1px solid var(--dark-border);
    padding: 6px 12px;
    border-radius: 999px;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    outline: none;
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  /* Main Container */
  .main-wrap {
    max-width: 1400px;
    width: 100%;
    margin: 0 auto;
    padding: 20px 24px 32px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  /* Buttons */
  .btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    border: 1px solid var(--border);
    background: var(--surface);
    color: var(--text);
    padding: 8px 15px;
    border-radius: 999px;
    font-size: 12.5px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
    white-space: nowrap;
  }
  .btn:hover {
    border-color: var(--dark);
    background: var(--surface-subtle);
  }
  .btn:disabled {
    opacity: 0.55;
    cursor: wait;
  }

  .btn-dark {
    background: var(--dark);
    color: #FFFFFF;
    border-color: var(--dark);
  }
  .btn-dark:hover {
    background: #27272A;
  }

  .btn-emerald {
    background: var(--emerald);
    color: #FFFFFF;
    border-color: var(--emerald);
  }
  .btn-emerald:hover {
    background: var(--emerald-dark);
    border-color: var(--emerald-dark);
  }

  .btn-outline-emerald {
    background: var(--surface);
    color: var(--emerald-dark);
    border-color: var(--emerald);
  }
  .btn-outline-emerald:hover {
    background: var(--emerald-soft);
  }

  /* Action Bar */
  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 18px;
    padding: 12px 16px;
    box-shadow: 0 4px 18px rgba(9, 9, 11, 0.03);
  }

  .toolbar-left {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .tabs {
    display: inline-flex;
    background: var(--surface-subtle);
    border: 1px solid var(--border);
    padding: 3px;
    border-radius: 999px;
    gap: 2px;
  }

  .tab {
    border: none;
    background: transparent;
    color: var(--muted);
    padding: 6px 13px;
    border-radius: 999px;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
  }
  .tab.active {
    background: var(--dark);
    color: #FFFFFF;
  }

  input, select {
    background: var(--surface);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 10px;
    padding: 8px 12px;
    font-size: 13px;
    outline: none;
    transition: border-color 0.15s ease;
  }
  input:focus {
    border-color: var(--emerald);
  }

  /* Cards */
  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 20px;
    overflow: hidden;
    box-shadow: 0 8px 26px rgba(9, 9, 11, 0.04);
  }

  .dark-panel {
    background: var(--dark);
    color: #FFFFFF;
    border: 1px solid var(--dark-border);
    border-radius: 20px;
    padding: 18px 22px;
    box-shadow: 0 16px 40px rgba(9, 9, 11, 0.10);
  }

  /* Free IP Pool Selector Panel */
  .free-panel {
    display: none;
    background: var(--surface);
    color: var(--text);
    border: 1px solid var(--dark);
    border-radius: 20px;
    padding: 18px 22px;
    gap: 14px;
    flex-direction: column;
    box-shadow: 0 8px 26px rgba(9, 9, 11, 0.05);
  }
  .free-top {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
  }
  .count-pills {
    display: inline-flex;
    gap: 6px;
    flex-wrap: wrap;
  }
  .count-pill {
    padding: 6px 13px;
    border-radius: 999px;
    font-size: 12px;
    font-weight: 700;
    font-family: var(--font-mono);
    border: 1px solid var(--border);
    background: var(--surface-subtle);
    color: var(--text);
    cursor: pointer;
    transition: all 0.15s ease;
  }
  .count-pill.active, .count-pill:hover {
    border-color: var(--dark);
    background: var(--dark);
    color: #FFFFFF;
  }

  .chip-list {
    display: flex;
    gap: 6px;
    flex-wrap: wrap;
    padding-top: 10px;
    border-top: 1px solid #F4F4F5;
  }
  .chip {
    font-family: var(--font-mono);
    font-size: 12px;
    font-weight: 700;
    padding: 6px 11px;
    border-radius: 999px;
    background: #FAFAFA;
    border: 1px solid var(--border);
    color: var(--dark);
    cursor: pointer;
    transition: all 0.12s ease;
  }
  .chip:hover {
    border-color: var(--dark);
    background: var(--dark);
    color: #FFFFFF;
  }

  /* Camera Table */
  .table-wrap {
    max-height: 390px;
    overflow-y: auto;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }

  th {
    position: sticky;
    top: 0;
    background: #FAFAFA;
    color: var(--muted-light);
    font-weight: 700;
    font-size: 11px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    text-align: left;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
    font-family: var(--font-mono);
  }

  td {
    padding: 12px 16px;
    border-bottom: 1px solid #F4F4F5;
    vertical-align: middle;
  }

  tbody tr {
    cursor: pointer;
    transition: background 0.12s ease;
  }
  tbody tr:hover {
    background: #FAFAFA;
  }
  tbody tr.selected {
    background: rgba(16, 185, 129, 0.08);
  }

  .zone-badge {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 999px;
    font-size: 11.5px;
    font-weight: 700;
  }
  .zone-local { background: var(--emerald-soft); color: var(--emerald-dark); }
  .zone-foreign { background: var(--amber-soft); color: var(--amber); }
  .zone-conflict { background: var(--red-soft); color: var(--red); }

  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    display: inline-block;
  }
  .dot-local { background: var(--emerald); }
  .dot-foreign { background: var(--amber); }
  .dot-conflict { background: var(--red); }

  .iface-tag {
    display: inline-block;
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 600;
    padding: 2px 8px;
    border-radius: 6px;
    background: var(--surface-subtle);
    border: 1px solid var(--border);
    color: var(--muted);
  }

  .mono {
    font-family: var(--font-mono);
    font-size: 12.5px;
  }

  /* Camera Control Workbench */
  .workbench {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .workbench-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 10px;
    border-bottom: 1px solid #27272A;
    padding-bottom: 10px;
  }

  .eyebrow {
    font-family: var(--font-mono);
    font-size: 11px;
    font-weight: 700;
    color: var(--emerald);
    letter-spacing: 0.04em;
    text-transform: uppercase;
  }

  .controls-row {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 14px;
  }

  .inputs-group {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
  }

  .field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .field label {
    font-size: 11px;
    font-weight: 600;
    color: #A1A1AA;
  }
  .field input {
    background: #18181B;
    border: 1px solid #3F3F46;
    color: #FFFFFF;
  }
  .field input:focus {
    border-color: var(--emerald);
  }

  .actions-group {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 7px;
  }

  .btn-wb {
    background: #18181B;
    color: #E4E4E7;
    border: 1px solid #3F3F46;
    padding: 8px 14px;
    border-radius: 999px;
    font-size: 12px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
  }
  .btn-wb:hover {
    border-color: var(--emerald);
    color: #FFFFFF;
  }
  .btn-wb-primary {
    background: var(--emerald);
    color: #09090B;
    border-color: var(--emerald);
    font-weight: 800;
  }
  .btn-wb-primary:hover {
    background: #34D399;
    color: #09090B;
  }

  /* Graphical Result Cards & Log */
  .bottom-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 16px;
  }
  @media (max-width: 960px) {
    .bottom-grid { grid-template-columns: 1fr; }
  }

  .diag-card {
    padding: 16px 20px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .diag-title {
    font-size: 13.5px;
    font-weight: 800;
    color: var(--dark);
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  pre#logBox {
    background: #18181B;
    border: 1px solid var(--dark-border);
    border-radius: 14px;
    padding: 12px 16px;
    height: 145px;
    overflow-y: auto;
    font-family: var(--font-mono);
    font-size: 12px;
    line-height: 1.55;
    color: #E4E4E7;
    white-space: pre-wrap;
  }

  /* Application Footer & Author Bar */
  .app-footer {
    border-top: 1px solid var(--border);
    background: #FFFFFF;
    padding: 12px 24px;
    margin-top: 20px;
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px;
    font-size: 12px;
    color: var(--muted);
  }
  .app-footer a {
    color: inherit;
    text-decoration: none;
    transition: color 0.12s ease;
  }
  .app-footer a:hover {
    color: var(--dark);
    text-decoration: underline;
  }
  .footer-author-btn {
    background: rgba(16, 185, 129, 0.1);
    border: 1px solid rgba(16, 185, 129, 0.3);
    color: var(--emerald-dark);
    font-size: 11.5px;
    font-weight: 700;
    padding: 5px 12px;
    border-radius: 999px;
    cursor: pointer;
    transition: all 0.15s ease;
    display: inline-flex;
    align-items: center;
    gap: 5px;
  }
  .footer-author-btn:hover {
    background: var(--emerald);
    color: #FFFFFF;
    border-color: var(--emerald);
  }

  /* Modals */
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(9, 9, 11, 0.45);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    display: none;
    align-items: center;
    justify-content: center;
    z-index: 300;
    padding: 20px;
  }
  .modal-box {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 22px;
    max-width: 720px;
    width: 100%;
    max-height: 88vh;
    overflow-y: auto;
    padding: 24px 28px;
    box-shadow: 0 24px 60px rgba(9, 9, 11, 0.22);
  }
  .modal-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-bottom: 14px;
    border-bottom: 1px solid var(--border);
    margin-bottom: 16px;
  }
  .modal-title {
    font-size: 17px;
    font-weight: 800;
    letter-spacing: -0.02em;
  }
  .help-item {
    padding: 11px 0;
    border-bottom: 1px solid #F4F4F5;
    font-size: 13px;
    line-height: 1.55;
    color: var(--muted);
  }
  .help-item:last-child { border-bottom: none; }
  .help-item b { color: var(--dark); font-size: 13.5px; display: block; margin-bottom: 3px; }
  .share-box {
    background: #FAFAFA;
    border: 1px solid var(--border);
    border-radius: 14px;
    padding: 14px 16px;
    margin-bottom: 12px;
  }
  .dl-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
    margin-top: 10px;
  }
  .dl-link {
    display: block;
    text-decoration: none;
    text-align: center;
    padding: 10px 14px;
    border-radius: 12px;
    border: 1px solid var(--border);
    background: #FFFFFF;
    color: var(--dark);
    font-size: 12.5px;
    font-weight: 700;
    transition: all 0.15s ease;
  }
  .dl-link:hover {
    border-color: var(--emerald);
    color: var(--emerald-dark);
  }
</style>
</head>
<body>

  <!-- ProCam Header with Cyber-Owl Logo & Dual-Network Selector -->
  <header class="header">
    <div class="brand-group">
      <div class="logo-box">
        <!-- CyberOwlMark SVG from ProCam -->
        <svg width="42" height="42" viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">
          <rect width="64" height="64" rx="15" fill="#09090B"/>
          <path d="M11 15L23 21L32 14L41 21L53 15L55 34C55 46.5 44.5 54.5 32 57C19.5 54.5 9 46.5 9 34L11 15Z" fill="#18181B" stroke="#E4E4E7" stroke-width="2" stroke-linejoin="round"/>
          <path d="M13 21L32 31L51 21" stroke="#FFFFFF" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
          <circle cx="22" cy="34" r="7.5" fill="#09090B" stroke="#FFFFFF" stroke-width="2"/>
          <circle cx="22" cy="34" r="3.5" fill="#09090B" stroke="#10B981" stroke-width="1.6"/>
          <circle cx="23.5" cy="32.5" r="1.7" fill="#10B981"/>
          <circle cx="42" cy="34" r="7.5" fill="#09090B" stroke="#FFFFFF" stroke-width="2"/>
          <circle cx="42" cy="34" r="3.5" fill="#09090B" stroke="#10B981" stroke-width="1.6"/>
          <circle cx="43.5" cy="32.5" r="1.7" fill="#10B981"/>
          <path d="M32 37L28.5 44L32 50L35.5 44L32 37Z" fill="#E4E4E7"/>
        </svg>
        <div>
          <div class="wordmark-main">PRO<span class="cam">CAM</span><span class="radar-pill">RADAR v4.1</span></div>
          <div class="wordmark-sub">СИСТЕМЫ ВИДЕОНАБЛЮДЕНИЯ · ИНЖЕНЕРНЫЙ КОМПЛЕКС</div>
        </div>
      </div>

      <div class="net-bar" id="netPillsBox">
        <span class="net-pill online"><span class="dot"></span>Загрузка сетей...</span>
      </div>

      <select class="iface-select" id="ifaceSelect" title="Выбор сетевого адаптера (при одновременной работе Wi-Fi и кабеля)">
        <option value="auto">Сеть: Авто (Кабель + Wi-Fi вместе)</option>
        <option value="wired">Только Кабель (Ethernet)</option>
        <option value="wifi">Только Wi-Fi (Роутер)</option>
      </select>
    </div>

    <div class="header-actions">
      <button class="btn btn-outline-emerald" onclick="openCalcModal()">Калькулятор (HDD · PoE · Объектив)</button>
      <button class="btn" onclick="cleanSubnets()">Очистить временные сети</button>
      <button class="btn" onclick="downloadAuditPDF()">Скачать PDF-отчет</button>
      <a class="btn" href="https://wa.me/77066066323?text=%D0%97%D0%B4%D1%80%D0%B0%D0%B2%D1%81%D1%82%D0%B2%D1%83%D0%B9%D1%82%D0%B5%2C%20ProCam!%20%D0%9F%D0%B8%D1%88%D1%83%20%D0%B8%D0%B7%20%D0%BF%D1%80%D0%BE%D0%B3%D1%80%D0%B0%D0%BC%D0%BC%D1%8B%20ProCam%20Radar%2C%20%D0%BD%D1%83%D0%B6%D0%BD%D0%B0%20%D0%BF%D0%BE%D0%BC%D0%BE%D1%89%D1%8C%20%D0%B8%D0%BD%D0%B6%D0%B5%D0%BD%D0%B5%D1%80%D0%B0%20%2F%20%D1%80%D0%B0%D1%81%D1%87%D1%91%D1%82%20%D0%BE%D0%B1%D0%BE%D1%80%D1%83%D0%B4%D0%BE%D0%B2%D0%B0%D0%BD%D0%B8%D1%8F." target="_blank" style="text-decoration:none; border-color:#10B981; color:#059669;">Помощь инженера / Заказ камер</a>
      <button class="btn btn-dark" onclick="openHelpModal()">Инструкция</button>
    </div>
  </header>

  <main class="main-wrap">

    <!-- Primary Graphical Toolbar with Clear Function Names -->
    <div class="toolbar">
      <div class="toolbar-left">
        <button class="btn btn-dark" id="scanBtn" style="padding:10px 20px; font-size:13.5px;" onclick="runScan()">
          <span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:#10B981;"></span>
          Найти камеры и NVR
        </button>
        <button class="btn" id="plugBtn" style="padding:10px 18px; font-size:13px; border-color:var(--dark);" onclick="runPlugScan()">
          Поиск по кабелю (Отключи → Включи)
        </button>
        <button class="btn" style="padding:10px 16px;" onclick="toggleFreePanel()">
          Подбор свободных IP
        </button>
        <button class="btn" style="padding:10px 16px;" onclick="openCalcModal()">
          Калькулятор HDD / PoE
        </button>
        <button class="btn" style="padding:10px 16px;" onclick="openActivateModal()">
          Пакетная активация и разведение
        </button>

        <div class="tabs">
          <button class="tab active" id="tab-all" onclick="setFilter('all')">Все (<span id="cnt-all">0</span>)</button>
          <button class="tab" id="tab-camera" onclick="setFilter('camera')">Камеры (<span id="cnt-camera">0</span>)</button>
          <button class="tab" id="tab-nvr" onclick="setFilter('nvr')">NVR / Регистраторы (<span id="cnt-nvr">0</span>)</button>
          <button class="tab" id="tab-local" onclick="setFilter('local')">Своя сеть (<span id="cnt-local">0</span>)</button>
          <button class="tab" id="tab-foreign" onclick="setFilter('foreign')">Чужая подсеть (<span id="cnt-foreign">0</span>)</button>
          <button class="tab" id="tab-conflict" onclick="setFilter('conflict')">Конфликты IP (<span id="cnt-conflict">0</span>)</button>
        </div>
      </div>

      <div>
        <input type="text" id="searchBox" placeholder="Фильтр по IP, MAC, модели..." style="width:230px; border-radius:999px;" oninput="renderTable()">
      </div>
    </div>

    <!-- Interactive Free IP Pool Finder with Count & Subnet Selection -->
    <div class="free-panel" id="freePanel">
      <div class="free-top">
        <div>
          <div class="eyebrow" style="color:var(--emerald-dark);">// ПОДБОР СВОБОДНЫХ IP-АДРЕСОВ В ПОДСЕТИ</div>
          <div style="font-size:14.5px; font-weight:800; color:var(--dark); margin-top:2px;">
            Выберите, сколько свободных IP-адресов нужно для новых камер:
          </div>
        </div>
        <div style="display:flex; align-items:center; gap:10px; flex-wrap:wrap;">
          <div class="count-pills">
            <button class="count-pill" onclick="setFreeCount(1)">1 камера</button>
            <button class="count-pill" onclick="setFreeCount(4)">4 камеры</button>
            <button class="count-pill active" id="defaultCountPill" onclick="setFreeCount(8)">8 камер</button>
            <button class="count-pill" onclick="setFreeCount(16)">16 камер</button>
            <button class="count-pill" onclick="setFreeCount(32)">32 камеры</button>
            <button class="count-pill" onclick="setFreeCount(64)">64 камеры</button>
          </div>
          <div style="display:flex; align-items:center; gap:6px;">
            <span style="font-size:12px; color:var(--muted); font-weight:600;">Свое число:</span>
            <input type="number" id="freeCountInput" value="8" min="1" max="245" class="mono" style="width:74px; font-weight:700;">
          </div>
          <div style="display:flex; align-items:center; gap:6px;">
            <span style="font-size:12px; color:var(--muted); font-weight:600;">Подсеть:</span>
            <input type="text" id="freeSubnetInput" placeholder="Авто (роутер)" class="mono" style="width:135px;">
          </div>
          <button class="btn btn-dark" onclick="findFreePool()">Подобрать свободные IP</button>
          <button class="btn" onclick="toggleFreePanel()">Скрыть</button>
        </div>
      </div>

      <div id="freeResultSummary" style="font-size:13px; color:var(--muted);">
        Укажите нужное количество камер (например, 8) и нажмите «Подобрать свободные IP».
      </div>
      <div class="chip-list" id="freePoolChips"></div>
      <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:8px; padding-top:8px; border-top:1px solid #F4F4F5;">
        <span style="font-size:12px; color:var(--muted);">Собираете систему видеонаблюдения? Подберем камеры Hikvision / HiWatch / Dahua и NVR со скидкой:</span>
        <a class="btn" href="https://wa.me/77066066323?text=%D0%97%D0%B4%D1%80%D0%B0%D0%B2%D1%81%D1%82%D0%B2%D1%83%D0%B9%D1%82%D0%B5%2C%20ProCam!%20%D0%9F%D0%BE%D0%B4%D0%B1%D0%B8%D1%80%D0%B0%D1%8E%20%D0%BA%D0%BE%D0%BC%D0%BF%D0%BB%D0%B5%D0%BA%D1%82%20%D0%B2%D0%B8%D0%B4%D0%B5%D0%BE%D0%BD%D0%B0%D0%B1%D0%BB%D1%8E%D0%B4%D0%B5%D0%BD%D0%B8%D1%8F%20%D1%87%D0%B5%D1%80%D0%B5%D0%B7%20ProCam%20Radar." target="_blank" style="text-decoration:none; font-size:12px; border-color:#10B981; color:#059669;">Рассчитать комплект в WhatsApp (+7 706 606-63-23)</a>
      </div>
    </div>

    <!-- Interactive Cable Plug Capture Banner -->
    <div class="card" id="plugBanner" style="display:none; padding:16px 20px; border-color:var(--emerald); background:var(--emerald-soft);">
      <div style="font-weight:800; font-size:14px; color:var(--emerald-dark); margin-bottom:4px;">
        Режим поиска по кабелю «Отключи → Включи» (осталось <span id="plugTimer">26</span> сек)
      </div>
      <div style="font-size:13px; color:var(--text); line-height:1.55;">
        <b>Шаг 1:</b> Отключите патч-корд (или питание PoE) искомой камеры на 3 секунды.<br>
        <b>Шаг 2:</b> Включите патч-корд обратно — ProCam Radar мгновенно перехватит пакет запуска камеры и построит к ней сетевой мост.
      </div>
    </div>

    <!-- Discovered Cameras Table -->
    <div class="card">
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th>Статус</th>
              <th>Адаптер</th>
              <th>IP-адрес</th>
              <th>MAC-адрес</th>
              <th>Производитель</th>
              <th>Модель / Серийный №</th>
              <th>Порты</th>
              <th>Сетевой мост</th>
              <th style="text-align:right;">Быстрые действия</th>
            </tr>
          </thead>
          <tbody id="camBody">
            <tr>
              <td colspan="9" style="text-align:center; padding: 44px 18px; color: var(--muted);">
                <div style="font-size:15px; font-weight:700; color:var(--dark); margin-bottom:6px;">
                  Готов к поиску IP-камер Hikvision, HiWatch, Dahua и ONVIF
                </div>
                <div style="margin-bottom:16px; font-size:13px;">
                  Программа работает одновременно с <b>Wi-Fi (интернет)</b> и <b>кабелем Ethernet</b> без разрыва соединения.
                </div>
                <div style="display:inline-flex; gap:10px; flex-wrap:wrap; justify-content:center;">
                  <button class="btn btn-dark" onclick="runScan()">Найти камеры в сети</button>
                  <button class="btn btn-outline-emerald" onclick="runPlugScan()">Поиск по кабелю (Отключи → Включи)</button>
                  <button class="btn" onclick="toggleFreePanel(); findFreePool();">Подбор свободных IP</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Dark ProCam Engineering Workbench for Selected Camera -->
    <div class="dark-panel workbench">
      <div class="workbench-head">
        <div>
          <span class="eyebrow">// ПАНЕЛЬ УПРАВЛЕНИЯ И ДИАГНОСТИКИ ВЫБРАННОЙ КАМЕРЫ</span>
        </div>
        <div style="font-size:12px; color:#A1A1AA;">
          Кликните по любой камере в таблице выше для автоматической подстановки IP и MAC
        </div>
      </div>

      <div class="controls-row">
        <div class="inputs-group">
          <div class="field">
            <label>Текущий IP камеры</label>
            <input class="mono" id="targetIP" value="192.168.1.64" style="width:140px;">
          </div>
          <div class="field">
            <label>Физический MAC-адрес</label>
            <input class="mono" id="targetMAC" placeholder="AA:BB:CC:11:22:33" style="width:162px;">
          </div>
          <div class="field">
            <label>Новый IP-адрес</label>
            <input class="mono" id="newIP" placeholder="192.168.1.101" style="width:140px;">
          </div>
          <div class="field">
            <label>Пароль камеры (опц.)</label>
            <input id="camPass" type="password" placeholder="Авто-подбор" style="width:135px;">
          </div>
        </div>

        <div class="actions-group">
          <button class="btn-wb btn-wb-primary" onclick="isolateAndOpen()">Открыть по MAC (при конфликте)</button>
          <button class="btn-wb" onclick="openWebSelected()">Открыть в браузере</button>
          <button class="btn-wb" onclick="playRTSPSelected()">Смотреть видео (RTSP)</button>
          <button class="btn-wb" onclick="snapSelected()">Сделать снимок</button>
          <button class="btn-wb" onclick="inspectCamera()">Паспорт камеры</button>
          <button class="btn-wb" onclick="cableTest()">Проверка кабеля и PoE</button>
          <button class="btn-wb" style="border-color:#10B981; color:#34D399;" onclick="changeIP()">Изменить IP-адрес</button>
        </div>
      </div>
    </div>

    <!-- Graphical Diagnostics Card + Event Log -->
    <div class="bottom-grid">
      <div class="card diag-card">
        <div class="diag-title">
          <span>Карточка диагностики (Паспорт камеры / Проверка кабеля RJ-45)</span>
          <span class="eyebrow" id="diagStatusBadge">ГОТОВ</span>
        </div>
        <div id="diagResultBox" style="font-size:13px; color:var(--muted); line-height:1.6;">
          Выберите камеру и нажмите <b>«Паспорт камеры»</b> (проверка активации, часов ONVIF, портов без пароля) или <b>«Проверка кабеля и PoE»</b> (тест обжима RJ-45 пакетами 64B и 1472B MTU 1500).
        </div>
      </div>

      <pre id="logBox">[ProCam Radar v4.0] Система готова. Одновременная работа Wi-Fi и Ethernet активна.</pre>
    </div>

    <!-- Sleek Architectural Footer & Author Bar -->
    <footer class="app-footer">
      <div style="display:flex; align-items:center; gap:8px;">
        <span class="status-dot dot-local"></span>
        <span style="font-weight:700; color:var(--dark);">ProCam Radar v4.1</span>
        <span>· Мульти-сетевой инженерный комплекс</span>
      </div>

      <div style="display:flex; align-items:center; gap:8px; flex-wrap:wrap;">
        <span>Автор и разработчик: <a href="https://yeee.kz" target="_blank" style="font-weight:700; color:var(--dark);">Ернияз</a> (<a href="https://yeee.kz" target="_blank" style="color:var(--emerald-dark); font-weight:600;">yeee.kz</a>)</span>
        <span>·</span>
        <a href="mailto:yeeerniyaz@gmail.com" class="mono" style="color:var(--muted);">yeeerniyaz@gmail.com</a>
        <span>·</span>
        <span class="mono">
          <a href="tel:+77066066323">+7 (706) 606-63-23</a> · <a href="tel:+77766066323">+7 (776) 606-63-23</a>
        </span>
      </div>

      <div>
        <button class="footer-author-btn" onclick="openAuthorModal()">
          <span>Все проекты & обо мне</span>
          <span>↗</span>
        </button>
      </div>
    </footer>

  </main>

  <!-- Модальное окно: Автор и все проекты -->
  <div class="modal-backdrop" id="authorModal" onclick="if(event.target===this)closeAuthorModal()">
    <div class="modal-box" style="max-width:820px;">
      <div class="modal-top">
        <div>
          <div class="eyebrow" style="color:var(--emerald-dark);">// АВТОР И ЭКОСИСТЕМА ПРОЕКТОВ</div>
          <div class="modal-title" style="margin-top:2px;">Ернияз Талғатулы · Разработчик & Инженер</div>
        </div>
        <button class="btn" onclick="closeAuthorModal()">Закрыть</button>
      </div>

      <!-- Карточка автора -->
      <div style="background:#18181B; border:1px solid #27272A; border-radius:14px; padding:16px 20px; color:#F4F4F5; margin-bottom:16px; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:14px;">
        <div style="display:flex; align-items:center; gap:14px;">
          <div style="width:52px; height:52px; border-radius:50%; background:linear-gradient(135deg, #10B981 0%, #059669 100%); display:flex; align-items:center; justify-content:center; color:#FFFFFF; font-weight:900; font-size:20px; box-shadow:0 4px 14px rgba(16, 185, 129, 0.4);">
            Y
          </div>
          <div>
            <div style="font-size:16px; font-weight:800; color:#FFFFFF;">Ернияз Талғатулы</div>
            <div style="font-size:12px; color:#A1A1AA; margin-top:2px;">Инженер систем безопасности & Fullstack-разработчик · Алматы, Казахстан</div>
            <div style="font-size:12px; margin-top:4px;">
              <a href="https://yeee.kz" target="_blank" style="color:#34D399; font-weight:700; text-decoration:none;">yeee.kz</a>
              <span style="color:#71717A;"> · </span>
              <a href="mailto:yeeerniyaz@gmail.com" style="color:#D4D4D8; text-decoration:none;" class="mono">yeeerniyaz@gmail.com</a>
            </div>
          </div>
        </div>
        <div style="display:flex; gap:8px;">
          <a class="btn-wb btn-wb-primary" href="https://wa.me/77066066323" target="_blank" style="text-decoration:none; padding:7px 14px; font-size:12px;">WhatsApp</a>
          <a class="btn-wb" href="https://t.me/yeeeerniyaz" target="_blank" style="text-decoration:none; padding:7px 14px; font-size:12px;">Telegram</a>
        </div>
      </div>

      <!-- Список проектов -->
      <div class="eyebrow" style="margin-bottom:8px;">// ВСЕ ПРОЕКТЫ И РАЗРАБОТКИ</div>
      <div style="display:grid; grid-template-columns:repeat(auto-fill, minmax(230px, 1fr)); gap:10px; margin-bottom:16px;">
        <div style="background:#FAFAFA; border:1px solid #E4E4E7; border-radius:12px; padding:12px 14px;">
          <div style="font-weight:700; font-size:13px; color:var(--dark);">ProCam CCTV</div>
          <div style="font-size:11px; color:var(--muted); margin-top:2px;">Монтаж IP-видеонаблюдения в Алматы (Hikvision, HiWatch, Dahua, PoE, ColorVu).</div>
          <a href="https://procam.yeee.kz" target="_blank" style="display:inline-block; margin-top:8px; font-size:11.5px; color:var(--emerald-dark); font-weight:700; text-decoration:none;">procam.yeee.kz ↗</a>
        </div>

        <div style="background:#FAFAFA; border:1px solid #E4E4E7; border-radius:12px; padding:12px 14px;">
          <div style="font-weight:700; font-size:13px; color:var(--dark);">ProCam Radar v4.1</div>
          <div style="font-size:11px; color:var(--muted); margin-top:2px;">Инженерный сканер по кабелю и Wi-Fi (/32), подбор свободных IP (1..245), PDF-паспорта.</div>
          <span style="display:inline-block; margin-top:8px; font-size:11.5px; color:#71717A; font-weight:600;">Текущая программа</span>
        </div>

        <div style="background:#FAFAFA; border:1px solid #E4E4E7; border-radius:12px; padding:12px 14px;">
          <div style="font-weight:700; font-size:13px; color:var(--dark);">ProCam Viewer</div>
          <div style="font-size:11px; color:var(--muted); margin-top:2px;">Клиентская станция видеонаблюдения, просмотр архива NVR с HDD и пакетная выгрузка.</div>
          <a href="https://procam.yeee.kz/#procam-radar" target="_blank" style="display:inline-block; margin-top:8px; font-size:11.5px; color:var(--emerald-dark); font-weight:700; text-decoration:none;">Скачать ПО ↗</a>
        </div>

        <div style="background:#FAFAFA; border:1px solid #E4E4E7; border-radius:12px; padding:12px 14px;">
          <div style="font-weight:700; font-size:13px; color:var(--dark);">Royal Banners (UKB)</div>
          <div style="font-size:11px; color:var(--muted); margin-top:2px;">Корпоративный портал и рекламная платформа наружной рекламы и полиграфии (royalbanners.yeee.kz).</div>
          <a href="https://royalbanners.yeee.kz/" target="_blank" style="display:inline-block; margin-top:8px; font-size:11.5px; color:var(--emerald-dark); font-weight:700; text-decoration:none;">royalbanners.yeee.kz ↗</a>
        </div>

        <div style="background:#FAFAFA; border:1px solid #E4E4E7; border-radius:12px; padding:12px 14px;">
          <div style="font-weight:700; font-size:13px; color:var(--dark);">TezTap Platform</div>
          <div style="font-size:11px; color:var(--muted); margin-top:2px;">SaaS-сервис для генерации страниц, визиток и микро-каталогов с заказами в WhatsApp.</div>
          <span style="display:inline-block; margin-top:8px; font-size:11.5px; color:#71717A; font-weight:600;">Внутренняя разработка</span>
        </div>

        <div style="background:#FAFAFA; border:1px solid #E4E4E7; border-radius:12px; padding:12px 14px;">
          <div style="font-weight:700; font-size:13px; color:var(--dark);">Justice Company</div>
          <div style="font-size:11px; color:var(--muted); margin-top:2px;">Минималистичный лендинг для юристов с интегрированной CMS и управлением через Telegram-бота.</div>
          <span style="display:inline-block; margin-top:8px; font-size:11.5px; color:#71717A; font-weight:600;">Внутренняя разработка</span>
        </div>

        <div style="background:#FAFAFA; border:1px solid #E4E4E7; border-radius:12px; padding:12px 14px;">
          <div style="font-weight:700; font-size:13px; color:var(--dark);">Smart Mirror VECTOR & IoT</div>
          <div style="font-size:11px; color:var(--muted); margin-top:2px;">Умные зеркала на React/Linux и промышленная автоматика Wiren Board (MQTT/Modbus).</div>
          <span style="display:inline-block; margin-top:8px; font-size:11.5px; color:#71717A; font-weight:600;">Внутренняя разработка</span>
        </div>
      </div>

      <!-- Контакты для связи -->
      <div style="background:var(--surface-subtle); border:1px solid var(--border); border-radius:12px; padding:12px 16px; display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px;">
        <div style="font-size:12px; color:var(--muted);">
          <b>Телефоны:</b> <a href="tel:+77066066323" style="color:var(--dark); font-weight:700;">+7 (706) 606-63-23</a> · <a href="tel:+77766066323" style="color:var(--dark); font-weight:700;">+7 (776) 606-63-23</a>
          <span style="margin:0 8px;">|</span>
          <b>Email:</b> <a href="mailto:yeeerniyaz@gmail.com" style="color:var(--dark); font-weight:600;">yeeerniyaz@gmail.com</a>
        </div>
        <a class="btn btn-outline-emerald" href="https://yeee.kz" target="_blank" style="text-decoration:none; font-size:12px; padding:6px 14px;">Портфолио yeee.kz ↗</a>
      </div>
    </div>
  </div>

  <!-- Модальное окно: Пакетная активация и разведение конфликтов -->
  <div class="modal-backdrop" id="activateModal" onclick="if(event.target===this)closeActivateModal()">
    <div class="modal-box">
      <div class="modal-top">
        <div class="modal-title">Пакетная активация камер и разведение конфликтов IP</div>
        <button class="btn" onclick="closeActivateModal()">Закрыть</button>
      </div>
      <div class="share-box">
        <div class="eyebrow" style="margin-bottom:4px;">// 01 · АКТИВАЦИЯ НОВЫХ КАМЕР «ИЗ КОРОБКИ»</div>
        <div style="font-size:13px; color:var(--muted); margin-bottom:10px;">
          Находит новые неактивированные камеры на заводских адресах (192.168.1.64, 192.168.1.108), задает им пароль и при необходимости раздает свободные IP по порядку (+1):
        </div>
        <div style="display:flex; gap:8px; flex-wrap:wrap; align-items:center;">
          <input type="text" id="batchNewPass" placeholder="Новый пароль (мин. 8 симв.)" style="flex:1; min-width:180px;">
          <input type="text" id="batchStartIP" placeholder="Стартовый IP (напр. 192.168.1.101)" class="mono" style="flex:1; min-width:180px;">
          <button class="btn btn-dark" onclick="runBatchActivate()">Активировать партию</button>
        </div>
      </div>

      <div class="share-box">
        <div class="eyebrow" style="margin-bottom:4px;">// 02 · АВТОМАТИЧЕСКОЕ РАЗВЕДЕНИЕ КОНФЛИКТА НА ОДНОМ IP</div>
        <div style="font-size:13px; color:var(--muted); margin-bottom:10px;">
          Если несколько камер висят на одном адресе (например, 192.168.1.64), программа по очереди изолирует каждую по MAC-адресу и переведет на стартовый IP, +1, +2 без отключения кабелей:
        </div>
        <div style="display:flex; gap:8px; flex-wrap:wrap; align-items:center;">
          <input type="text" id="resolveConflictIP" value="192.168.1.64" class="mono" style="width:140px;">
          <input type="text" id="resolveStartIP" placeholder="Свободный IP (192.168.1.101)" class="mono" style="flex:1; min-width:170px;">
          <input type="password" id="resolvePass" placeholder="Пароль камер" style="width:140px;">
          <button class="btn btn-emerald" onclick="runBatchResolve()">Развести конфликт</button>
        </div>
      </div>
    </div>
  </div>

  <!-- Модальное окно: Инструкция -->
  <div class="modal-backdrop" id="helpModal" onclick="if(event.target===this)closeHelpModal()">
    <div class="modal-box">
      <div class="modal-top">
        <div class="modal-title">Инструкция по графическому приложению ProCam Radar</div>
        <div style="display:flex; gap:8px;">
          <a class="btn btn-emerald" href="/download/manual" target="_blank" style="text-decoration:none;">Открыть полную PDF-инструкцию</a>
          <button class="btn" onclick="closeHelpModal()">Закрыть</button>
        </div>
      </div>
      <div class="help-item">
        <b>1. Одновременная работа Wi-Fi и кабеля Ethernet без потери интернета</b>
        Вы можете быть подключены по Wi-Fi к роутеру (интернет) и одновременно воткнуть патч-корд камеры напрямую в ноутбук. В верхней панели выберите <b>«Сеть: Авто (Кабель + Wi-Fi вместе)»</b>. Программа сама создаст точечный маршрут /32 к камере на кабеле, не обрывая ваш Wi-Fi!
      </div>
      <div class="help-item">
        <b>2. Найти камеры в сети и Поиск по кабелю (Отключи → Включи)</b>
        Кнопка <b>«Найти камеры в сети»</b> за 4 секунды находит все камеры в родной и чужих подсетях (ONVIF, Hikvision SADP, Dahua). Чтобы определить одну конкретную камеру на объекте — нажмите <b>«Поиск по кабелю (Отключи → Включи)»</b> и переткните её патч-корд на 3 секунды.
      </div>
      <div class="help-item">
        <b>3. Подбор свободных IP с выбором нужного количества камер</b>
        Нажмите <b>«Подбор свободных IP»</b>, выберите нужное количество камер (1, 4, 8, 16, 32, 64 или введите любое число вручную) — программа проверит все 254 адреса подсети и выдаст готовый непрерывный диапазон.
      </div>
      <div class="help-item">
        <b>4. Открыть по MAC (при конфликте IP) без пароля</b>
        Если несколько камер сидят на одном IP (например, 192.168.1.64), выберите камеру в таблице и нажмите <b>«Открыть по MAC (при конфликте)»</b>. Вторая камера блокируется на уровне сетевой карты, и в браузере открывается именно выбранная камера.
      </div>
      <div class="help-item">
        <b>5. Паспорт камеры, Проверка кабеля и Инженерный калькулятор (HDD · PoE · Объектив)</b>
        Кнопка <b>«Паспорт камеры»</b> без пароля показывает статус активации и точность часов ONVIF. Кнопка <b>«Проверка кабеля и PoE»</b> тестирует линию пакетами 64B и 1472B (MTU 1500). Кнопка <b>«Калькулятор (HDD · PoE · Объектив)»</b> рассчитывает объем жесткого диска, мощность PoE-коммутатора, входящий поток NVR и угол обзора объектива (2.8 / 4 / 6 / 12 мм).
      </div>
      <div class="help-item">
        <b>6. Разработчик комплекса, техническая поддержка и контакты</b>
        Автор программы ProCam Radar: <b>Ернияз Талғатулы</b> (инженер систем безопасности и разработчик ПО).<br>
        Контакты для связи, заявок и консультаций:<br>
        • Телефон / WhatsApp: <a href="https://wa.me/77066066323" target="_blank" style="color:var(--emerald-dark); font-weight:700;">+7 (706) 606-63-23</a> · <a href="tel:+77766066323" style="color:var(--emerald-dark); font-weight:700;">+7 (776) 606-63-23</a><br>
        • Email: <a href="mailto:yeeerniyaz@gmail.com" style="color:var(--dark); font-weight:600;">yeeerniyaz@gmail.com</a><br>
        • Персональный сайт и все проекты: <a href="https://yeee.kz" target="_blank" style="color:var(--emerald-dark); font-weight:700; text-decoration:underline;">yeee.kz</a> (ProCam CCTV, ProCam Radar, ProCam Viewer, Royal Banners — royalbanners.yeee.kz, TezTap, Justice Company).
      </div>
    </div>
  </div>

  <!-- Модальное окно: Инженерный калькулятор (HDD + PoE + Объектив) -->
  <div class="modal-backdrop" id="calcModal" onclick="if(event.target===this)closeCalcModal()">
    <div class="modal-box" style="max-width:860px;">
      <div class="modal-top">
        <div>
          <div class="eyebrow" style="color:var(--emerald-dark);">// ИНЖЕНЕРНЫЙ КАЛЬКУЛЯТОР ВИДЕОНАБЛЮДЕНИЯ PROCAM</div>
          <div class="modal-title" style="margin-top:2px;">Расчет жесткого диска (HDD), трафика PoE/NVR и объектива камер</div>
        </div>
        <button class="btn" onclick="closeCalcModal()">Закрыть</button>
      </div>

      <!-- Вкладки калькулятора -->
      <div class="tabs" style="margin-bottom:16px; width:100%; justify-content:space-between;">
        <button class="tab active" id="ctab-hdd" style="flex:1; padding:9px 14px;" onclick="setCalcTab('hdd')">1. Жесткий диск и Архив (HDD)</button>
        <button class="tab" id="ctab-poe" style="flex:1; padding:9px 14px;" onclick="setCalcTab('poe')">2. Сеть, NVR и Бюджет PoE</button>
        <button class="tab" id="ctab-lens" style="flex:1; padding:9px 14px;" onclick="setCalcTab('lens')">3. Объектив (2.8–12 мм) и DORI</button>
      </div>

      <!-- Вкладка 1: HDD & Архив -->
      <div id="cpanel-hdd">
        <div class="share-box" style="display:flex; flex-direction:column; gap:12px;">
          <div style="display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:10px;">
            <div>
              <div style="font-size:12px; font-weight:700; color:var(--dark); margin-bottom:4px;">Количество камер в системе:</div>
              <div class="count-pills">
                <button class="count-pill" onclick="setCalcCams(1)">1</button>
                <button class="count-pill" onclick="setCalcCams(4)">4</button>
                <button class="count-pill active" onclick="setCalcCams(8)">8</button>
                <button class="count-pill" onclick="setCalcCams(16)">16</button>
                <button class="count-pill" onclick="setCalcCams(32)">32</button>
                <button class="count-pill" onclick="setCalcCams(64)">64</button>
              </div>
            </div>
            <div style="display:flex; align-items:center; gap:8px;">
              <input type="number" id="calcCams" value="8" min="1" max="512" class="mono" style="width:82px; font-weight:800;" oninput="calcUpdate()">
              <button class="btn" style="padding:7px 12px; font-size:11.5px;" onclick="useScannedCamsInCalc()">Взять из найденных в сети</button>
            </div>
          </div>

          <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(180px, 1fr)); gap:10px;">
            <div>
              <div style="font-size:11.5px; font-weight:700; color:var(--muted); margin-bottom:4px;">Разрешение камер:</div>
              <select id="calcRes" style="width:100%;" onchange="calcUpdate()">
                <option value="2">2 МП (1080p Full HD)</option>
                <option value="4" selected>4 МП (2K QHD — Стандарт)</option>
                <option value="5">5 МП (Super HD)</option>
                <option value="6">6 МП (Ultra HD)</option>
                <option value="8">8 МП (4K Ultra HD)</option>
              </select>
            </div>
            <div>
              <div style="font-size:11.5px; font-weight:700; color:var(--muted); margin-bottom:4px;">Кодек сжатия:</div>
              <select id="calcCodec" style="width:100%;" onchange="calcUpdate()">
                <option value="h265plus" selected>H.265+ / Smart H.265+ (Экономия 65%)</option>
                <option value="h265">H.265 (Стандартный HEVC)</option>
                <option value="h264">H.264 (Старый кодек)</option>
              </select>
            </div>
            <div>
              <div style="font-size:11.5px; font-weight:700; color:var(--muted); margin-bottom:4px;">Частота кадров (FPS):</div>
              <select id="calcFPS" style="width:100%;" onchange="calcUpdate()">
                <option value="15">15 к/с (Эконом)</option>
                <option value="20" selected>20 к/с (Оптимально)</option>
                <option value="25">25 к/с (Плавное видео)</option>
              </select>
            </div>
            <div>
              <div style="font-size:11.5px; font-weight:700; color:var(--muted); margin-bottom:4px;">Режим записи:</div>
              <select id="calcMode" style="width:100%;" onchange="calcUpdate()">
                <option value="1.0" selected>Постоянная запись 24/7 (100%)</option>
                <option value="0.7">По движению: Магазин / Склад (~70%)</option>
                <option value="0.45">По движению: Дом / Офис (~45%)</option>
              </select>
            </div>
          </div>

          <div style="display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:10px;">
            <div>
              <div style="font-size:12px; font-weight:700; color:var(--dark); margin-bottom:4px;">Глубина хранения архива (дней):</div>
              <div class="count-pills">
                <button class="count-pill" onclick="setCalcDays(7)">7 дней</button>
                <button class="count-pill" onclick="setCalcDays(14)">14 дней</button>
                <button class="count-pill active" onclick="setCalcDays(30)">30 дней (1 мес)</button>
                <button class="count-pill" onclick="setCalcDays(60)">60 дней</button>
                <button class="count-pill" onclick="setCalcDays(90)">90 дней</button>
              </div>
            </div>
            <div style="display:flex; align-items:center; gap:6px;">
              <span style="font-size:12px; color:var(--muted); font-weight:600;">Дней:</span>
              <input type="number" id="calcDays" value="30" min="1" max="365" class="mono" style="width:76px; font-weight:800;" oninput="calcUpdate()">
            </div>
          </div>
        </div>
      </div>

      <!-- Вкладка 2: PoE, коммутатор и трафик NVR -->
      <div id="cpanel-poe" style="display:none;">
        <div class="share-box" style="display:flex; flex-direction:column; gap:12px;">
          <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(220px, 1fr)); gap:10px;">
            <div>
              <div style="font-size:11.5px; font-weight:700; color:var(--muted); margin-bottom:4px;">Тип подсветки и питания камер:</div>
              <select id="calcCamPower" style="width:100%;" onchange="calcUpdate()">
                <option value="6.5" selected>Обычные ИК-камеры (802.3af — до 6.5 Вт)</option>
                <option value="10.5">ColorVu / Dual-Light LED (до 10.5 Вт)</option>
                <option value="22.0">Поворотные PTZ-купола (802.3at PoE+ — до 22 Вт)</option>
              </select>
            </div>
            <div>
              <div style="font-size:11.5px; font-weight:700; color:var(--muted); margin-bottom:4px;">Максимальная длина кабеля UTP (м):</div>
              <input type="number" id="calcCableLen" value="65" min="5" max="300" class="mono" style="width:100%; font-weight:800;" oninput="calcUpdate()">
            </div>
            <div>
              <div style="font-size:11.5px; font-weight:700; color:var(--muted); margin-bottom:4px;">Материал витой пары UTP Cat5e:</div>
              <select id="calcCableMat" style="width:100%;" onchange="calcUpdate()">
                <option value="cu" selected>Чистая медь (100% Cu, 0.51 мм — ГОСТ)</option>
                <option value="cca">Омедненный алюминий (CCA — бюджетный)</option>
              </select>
            </div>
          </div>
        </div>
      </div>

      <!-- Вкладка 3: Объектив (2.8 / 4 / 6 / 12 мм) и DORI -->
      <div id="cpanel-lens" style="display:none;">
        <div class="share-box" style="display:flex; flex-direction:column; gap:12px;">
          <div style="display:grid; grid-template-columns:repeat(auto-fit, minmax(220px, 1fr)); gap:10px;">
            <div>
              <div style="font-size:11.5px; font-weight:700; color:var(--muted); margin-bottom:4px;">Фокусное расстояние объектива:</div>
              <select id="calcFocal" style="width:100%;" onchange="calcUpdate()">
                <option value="2.8" selected>2.8 мм — Широкий угол ~103° (Двор / Помещение)</option>
                <option value="4.0">4.0 мм — Средний угол ~83° (Вход / Калитка / Коридор)</option>
                <option value="6.0">6.0 мм — Направленный ~54° (Парковка / Периметр)</option>
                <option value="12.0">12.0 мм — Дальнобойный ~25° (Шлагбаум / Ворота)</option>
              </select>
            </div>
            <div>
              <div style="font-size:11.5px; font-weight:700; color:var(--muted); margin-bottom:4px;">Расстояние до объекта (метров):</div>
              <input type="number" id="calcDist" value="10" min="1" max="150" class="mono" style="width:100%; font-weight:800;" oninput="calcUpdate()">
            </div>
          </div>
        </div>
      </div>

      <!-- Сводное табло результатов (всегда видно) -->
      <div class="dark-panel" style="padding:18px 22px; margin-top:4px;">
        <div class="eyebrow" style="margin-bottom:10px;">// ИТОГОВЫЙ ИНЖЕНЕРНЫЙ РАСЧЕТ СИСТЕМЫ</div>
        <div id="calcSummaryGrid" style="display:grid; grid-template-columns:repeat(auto-fit, minmax(175px, 1fr)); gap:12px; margin-bottom:14px;"></div>
        <div id="calcDetailsBox" style="background:#18181B; border:1px solid #27272A; border-radius:12px; padding:12px 14px; font-size:12.5px; color:#E4E4E7; line-height:1.6;"></div>
        <div style="display:flex; justify-content:space-between; align-items:center; flex-wrap:wrap; gap:10px; margin-top:14px; padding-top:12px; border-top:1px solid #27272A;">
          <button class="btn-wb" onclick="copyCalcSummary()">Скопировать расчет для клиента</button>
          <a class="btn-wb btn-wb-primary" id="calcWhatsappBtn" href="https://wa.me/77066066323" target="_blank" style="text-decoration:none;">Заказать комплект по расчету в WhatsApp</a>
        </div>
      </div>
    </div>
  </div>

<script>
let allCameras = [];
let currentFilter = 'all';
let selectedIP = '';
let selectedMAC = '';
let lastCalcPlainText = '';

function openHelpModal() { document.getElementById('helpModal').style.display = 'flex'; }
function closeHelpModal() { document.getElementById('helpModal').style.display = 'none'; }
function openActivateModal() { document.getElementById('activateModal').style.display = 'flex'; }
function closeActivateModal() { document.getElementById('activateModal').style.display = 'none'; }
function openCalcModal() {
  document.getElementById('calcModal').style.display = 'flex';
  calcUpdate();
}
function closeCalcModal() { document.getElementById('calcModal').style.display = 'none'; }
function openAuthorModal() { document.getElementById('authorModal').style.display = 'flex'; }
function closeAuthorModal() { document.getElementById('authorModal').style.display = 'none'; }

function setCalcTab(tab) {
  ['hdd', 'poe', 'lens'].forEach(k => {
    document.getElementById('ctab-' + k).classList.toggle('active', k === tab);
    document.getElementById('cpanel-' + k).style.display = (k === tab) ? 'block' : 'none';
  });
  calcUpdate();
}

function setCalcCams(n) {
  document.getElementById('calcCams').value = n;
  calcUpdate();
}

function setCalcDays(d) {
  document.getElementById('calcDays').value = d;
  calcUpdate();
}

function useScannedCamsInCalc() {
  const camOnly = allCameras.filter(c => c.device_type !== 'nvr').length;
  const n = camOnly > 0 ? camOnly : (allCameras.length || 8);
  document.getElementById('calcCams').value = n;
  calcUpdate();
}

function calcUpdate() {
  const cams = Math.max(1, parseInt(document.getElementById('calcCams').value || '8', 10));
  const resMP = parseFloat(document.getElementById('calcRes').value || '4');
  const codec = document.getElementById('calcCodec').value || 'h265plus';
  const fps = parseInt(document.getElementById('calcFPS').value || '20', 10);
  const modeFactor = parseFloat(document.getElementById('calcMode').value || '1.0');
  const days = Math.max(1, parseInt(document.getElementById('calcDays').value || '30', 10));

  // Базовый битрейт (Мбит/с) для 25 к/с на H.265
  const baseMbpsMap = { 2: 2.2, 4: 3.8, 5: 4.5, 6: 5.2, 8: 7.5 };
  let mbpsPerCam = baseMbpsMap[resMP] || 3.8;
  if (codec === 'h265plus') mbpsPerCam *= 0.52;
  else if (codec === 'h264') mbpsPerCam *= 1.85;
  mbpsPerCam *= (fps / 25.0);
  mbpsPerCam = Math.round(mbpsPerCam * 100) / 100;

  const totalMbps = Math.round(mbpsPerCam * cams * 10) / 10;
  const gbPerCamDay = (mbpsPerCam * 3600 * 24) / 8 / 1024 * modeFactor;
  const totalGBPerDay = gbPerCamDay * cams;
  const rawTotalTB = (totalGBPerDay * days) / 1024;
  // С учетом реальной форматированной емкости HDD (~90.9%)
  const diskNominalTB = rawTotalTB / 0.909;

  let recDiskText = '1× 1 ТБ';
  const stdSizes = [1, 2, 4, 6, 8, 10, 12, 16, 18, 20];
  let foundSingle = stdSizes.find(s => s >= diskNominalTB);
  if (foundSingle) {
    recDiskText = '1× ' + foundSingle + ' ТБ (WD Purple / SkyHawk)';
  } else {
    const bays = Math.ceil(diskNominalTB / 10);
    const perBay = stdSizes.find(s => s * bays >= diskNominalTB) || 10;
    recDiskText = bays + '× ' + perBay + ' ТБ (суммарно ' + (bays * perBay) + ' ТБ)';
  }

  // PoE и NVR
  const camWatts = parseFloat(document.getElementById('calcCamPower').value || '6.5');
  const cableLen = Math.max(1, parseInt(document.getElementById('calcCableLen').value || '65', 10));
  const cableMat = document.getElementById('calcCableMat').value || 'cu';
  const totalPoEWatts = Math.round(cams * camWatts);
  const recSwitchWatts = Math.ceil((totalPoEWatts * 1.3) / 10) * 10;

  let nvrChannels = 4;
  if (cams > 32) nvrChannels = 64;
  else if (cams > 16) nvrChannels = 32;
  else if (cams > 8) nvrChannels = 16;
  else if (cams > 4) nvrChannels = 8;
  const recNvrBandwidth = Math.max(40, Math.ceil((totalMbps * 1.5) / 40) * 40);

  // Расчет падения напряжения на витой паре (петля 2 жилы / пара)
  const loopOhmPer100m = (cableMat === 'cu') ? 18.8 : 29.5;
  const loopR = (loopOhmPer100m * cableLen) / 100.0;
  const currentA = camWatts / 48.0;
  const vDrop = Math.round(currentA * loopR * 10) / 10;
  const vAtCam = Math.round((48.0 - vDrop) * 10) / 10;
  let cableVerdict = 'Норма (Гигабит / 100 Мбит/с без потерь)';
  if (cableLen > 100 && cableLen <= 250) {
    cableVerdict = 'Длина > 100 м: включите режим PoE Extend (10 Мбит/с до 250 м)';
  } else if (cableLen > 250 || vAtCam < 37.0) {
    cableVerdict = 'ВНИМАНИЕ: Критическая длина/просадка! Нужен PoE-удлинитель или оптика';
  } else if (cableMat === 'cca' && cableLen > 70) {
    cableVerdict = 'Риск просадки PoE ночью на кабеле CCA > 70 м (рекомендуется медь Cu)';
  }

  // Объектив и DORI
  const focal = parseFloat(document.getElementById('calcFocal').value || '2.8');
  const dist = Math.max(1, parseFloat(document.getElementById('calcDist').value || '10'));
  const fovMap = { 2.8: 103, 4.0: 83, 6.0: 54, 12.0: 25 };
  const angleDeg = fovMap[focal] || 90;
  const sceneWidthM = Math.round(2 * dist * Math.tan((angleDeg * Math.PI) / 360) * 10) / 10;
  const horizPxMap = { 2: 1920, 4: 2560, 5: 2880, 6: 3200, 8: 3840 };
  const hPx = horizPxMap[resMP] || 2560;
  const ppm = Math.round(hPx / Math.max(0.5, sceneWidthM));

  // DORI дистанции (Detection 25 ppm, Observation 62 ppm, Recognition 125 ppm, Identification 250 ppm)
  const factorW = 2 * Math.tan((angleDeg * Math.PI) / 360);
  const doriId = Math.round((hPx / 250 / factorW) * 10) / 10;
  const doriRec = Math.round((hPx / 125 / factorW) * 10) / 10;
  const doriObs = Math.round((hPx / 62 / factorW) * 10) / 10;
  const doriDet = Math.round((hPx / 25 / factorW) * 10) / 10;

  document.getElementById('calcSummaryGrid').innerHTML =
    '<div style="background:#18181B;border:1px solid #27272A;border-radius:12px;padding:10px 12px;">' +
      '<div style="font-size:11px;color:#A1A1AA;">Чистый архив (' + days + ' дн.)</div>' +
      '<div class="mono" style="font-size:18px;font-weight:900;color:#10B981;margin-top:2px;">' + rawTotalTB.toFixed(2) + ' ТБ</div>' +
      '<div style="font-size:11px;color:#E4E4E7;margin-top:2px;">' + Math.round(totalGBPerDay) + ' ГБ / сутки</div>' +
    '</div>' +
    '<div style="background:#18181B;border:1px solid #27272A;border-radius:12px;padding:10px 12px;">' +
      '<div style="font-size:11px;color:#A1A1AA;">Рекомендуемый HDD</div>' +
      '<div style="font-size:15px;font-weight:800;color:#FFFFFF;margin-top:4px;">' + recDiskText + '</div>' +
      '<div style="font-size:11px;color:#A1A1AA;margin-top:2px;">С учетом реальной емкости</div>' +
    '</div>' +
    '<div style="background:#18181B;border:1px solid #27272A;border-radius:12px;padding:10px 12px;">' +
      '<div style="font-size:11px;color:#A1A1AA;">Поток и Регистратор</div>' +
      '<div class="mono" style="font-size:16px;font-weight:900;color:#FFFFFF;margin-top:2px;">' + totalMbps + ' Мбит/с</div>' +
      '<div style="font-size:11px;color:#10B981;margin-top:2px;">NVR: ' + nvrChannels + '-кан. (от ' + recNvrBandwidth + ' Мбит/с)</div>' +
    '</div>' +
    '<div style="background:#18181B;border:1px solid #27272A;border-radius:12px;padding:10px 12px;">' +
      '<div style="font-size:11px;color:#A1A1AA;">Бюджет PoE коммутатора</div>' +
      '<div class="mono" style="font-size:16px;font-weight:900;color:#F59E0B;margin-top:2px;">' + totalPoEWatts + ' Вт (мин. БП ' + recSwitchWatts + ' Вт)</div>' +
      '<div style="font-size:11px;color:#E4E4E7;margin-top:2px;">На камере (' + cableLen + 'м): ~' + vAtCam + ' В</div>' +
    '</div>';

  document.getElementById('calcDetailsBox').innerHTML =
    '<b>• Детализация архива и трафика:</b> 1 камера (' + resMP + ' МП, ' + codec.toUpperCase() + ', ' + fps + ' к/с) = <b>' + mbpsPerCam + ' Мбит/с</b> (' + gbPerCamDay.toFixed(1) + ' ГБ/сутки). Все ' + cams + ' шт. = <b>' + Math.round(totalGBPerDay) + ' ГБ/сутки</b>.<br>' +
    '<b>• Кабельная трасса (' + cableLen + ' м):</b> ' + cableVerdict + '.<br>' +
    '<b>• Объектив ' + focal + ' мм (~' + angleDeg + '°) на дистанции ' + dist + ' м:</b> ширина обзора <b>' + sceneWidthM + ' м</b> (плотность <b>' + ppm + ' пикс/м</b>). Чтение лица/номера (DORI ID): до <b>' + doriId + ' м</b> · Узнавание знакомого: до <b>' + doriRec + ' м</b> · Обнаружение человека: до <b>' + doriDet + ' м</b>.';

  lastCalcPlainText =
    'Расчет системы видеонаблюдения ProCam Radar:\n' +
    '• Камер: ' + cams + ' шт. (' + resMP + ' МП, ' + codec.toUpperCase() + ', ' + fps + ' к/с)\n' +
    '• Архив на ' + days + ' дн.: ' + rawTotalTB.toFixed(2) + ' ТБ -> Рекомендуемый HDD: ' + recDiskText + '\n' +
    '• Регистратор (NVR): ' + nvrChannels + '-канальный (входящий поток ' + totalMbps + ' Мбит/с, запас от ' + recNvrBandwidth + ' Мбит/с)\n' +
    '• PoE-коммутатор: потребление ' + totalPoEWatts + ' Вт (рекомендуемый БП от ' + recSwitchWatts + ' Вт)\n' +
    '• Объектив ' + focal + ' мм (' + angleDeg + '°): захват на ' + dist + ' м = ' + sceneWidthM + ' м в ширину (чтение лица до ' + doriId + ' м).';

  const waUrl = 'https://wa.me/77066066323?text=' + encodeURIComponent('Здравствуйте, ProCam! Нужен комплект по расчету из ProCam Radar:\n' + lastCalcPlainText);
  document.getElementById('calcWhatsappBtn').href = waUrl;
}

function copyCalcSummary() {
  if (!lastCalcPlainText) calcUpdate();
  navigator.clipboard.writeText(lastCalcPlainText).then(() => {
    log("Инженерный расчет скопирован в буфер обмена.");
    alert("Расчет скопирован в буфер обмена! Можете вставить его в WhatsApp или КП клиенту.");
  });
}

function toggleFreePanel() {
  const p = document.getElementById('freePanel');
  p.style.display = (p.style.display === 'flex') ? 'none' : 'flex';
}

function setFreeCount(n) {
  document.getElementById('freeCountInput').value = n;
  document.querySelectorAll('.count-pill').forEach(btn => {
    btn.classList.toggle('active', btn.textContent.indexOf(String(n) + ' ') === 0);
  });
  findFreePool();
}

function log(msg) {
  const b = document.getElementById('logBox');
  const now = new Date().toLocaleTimeString('ru-RU');
  b.textContent += "\n[" + now + "] " + msg;
  b.scrollTop = b.scrollHeight;
}

function setDiagHTML(badgeText, htmlContent) {
  document.getElementById('diagStatusBadge').textContent = badgeText;
  document.getElementById('diagResultBox').innerHTML = htmlContent;
}

async function loadSysInfo() {
  try {
    const r = await fetch('/api/info');
    const info = await r.json();
    const pillsBox = document.getElementById('netPillsBox');
    if (info.interfaces && info.interfaces.length > 0) {
      pillsBox.innerHTML = info.interfaces.map(inf => {
        const cls = inf.carrier ? 'net-pill online' : 'net-pill';
        return '<span class="' + cls + '"><span class="dot"></span>' + inf.label + '</span>';
      }).join('');
    }
    if (info.subnet && !document.getElementById('freeSubnetInput').value) {
      document.getElementById('freeSubnetInput').value = info.subnet;
    }
  } catch (e) {}
}

function setFilter(f) {
  currentFilter = f;
  ['all', 'camera', 'nvr', 'local', 'foreign', 'conflict'].forEach(k => {
    const el = document.getElementById('tab-' + k);
    if (el) el.classList.toggle('active', k === f);
  });
  renderTable();
}

function selectCamera(c) {
  selectedIP = c.ip;
  selectedMAC = c.mac || '';
  document.getElementById('targetIP').value = c.ip;
  document.getElementById('targetMAC').value = c.mac || '';
  renderTable();
}

function renderTable() {
  const q = (document.getElementById('searchBox').value || '').toLowerCase().trim();
  const tb = document.getElementById('camBody');

  let localCnt = 0, foreignCnt = 0, conflictCnt = 0, camCnt = 0, nvrCnt = 0;
  allCameras.forEach(c => {
    if (c.device_type === 'nvr') nvrCnt++;
    else camCnt++;
    if (c.zone_type === 'conflict') conflictCnt++;
    else if (c.zone_type === 'foreign') foreignCnt++;
    else localCnt++;
  });

  document.getElementById('cnt-all').textContent = allCameras.length;
  document.getElementById('cnt-camera').textContent = camCnt;
  document.getElementById('cnt-nvr').textContent = nvrCnt;
  document.getElementById('cnt-local').textContent = localCnt;
  document.getElementById('cnt-foreign').textContent = foreignCnt;
  document.getElementById('cnt-conflict').textContent = conflictCnt;

  const filtered = allCameras.filter(c => {
    if (currentFilter === 'camera' && c.device_type === 'nvr') return false;
    if (currentFilter === 'nvr' && c.device_type !== 'nvr') return false;
    if (['local', 'foreign', 'conflict'].includes(currentFilter) && c.zone_type !== currentFilter) return false;
    if (!q) return true;
    const hay = [c.ip, c.mac, c.vendor, c.model, c.serial, c.device_label, c.iface, c.subnet_alias].join(' ').toLowerCase();
    return hay.includes(q);
  });

  if (!filtered.length) {
    tb.innerHTML = '<tr><td colspan="9" style="text-align:center; padding: 36px 16px; color: var(--muted);">Устройства по выбранному фильтру не найдены. Нажмите «Найти камеры и NVR».</td></tr>';
    return;
  }

  tb.innerHTML = '';
  filtered.forEach(c => {
    const tr = document.createElement('tr');
    if (c.ip === selectedIP && (!selectedMAC || c.mac === selectedMAC)) tr.classList.add('selected');
    tr.onclick = () => selectCamera(c);

    let zoneCls = 'zone-local', dotCls = 'dot-local';
    if (c.zone_type === 'conflict') { zoneCls = 'zone-conflict'; dotCls = 'dot-conflict'; }
    else if (c.zone_type === 'foreign') { zoneCls = 'zone-foreign'; dotCls = 'dot-foreign'; }

    const isNVR = (c.device_type === 'nvr');
    const roleBadge = isNVR
      ? '<span style="display:inline-block;padding:2px 7px;border-radius:6px;font-size:10.5px;font-weight:800;font-family:var(--font-mono);background:#EEF2FF;color:#4338CA;border:1px solid #C7D2FE;margin-right:6px;">NVR / РЕГИСТРАТОР</span>'
      : '<span style="display:inline-block;padding:2px 7px;border-radius:6px;font-size:10.5px;font-weight:800;font-family:var(--font-mono);background:#F0FDF4;color:#15803D;border:1px solid #BBF7D0;margin-right:6px;">IP-КАМЕРА</span>';

    const ifaceLabel = (c.iface_kind === 'wifi' ? 'Wi-Fi · ' : 'Кабель · ') + (c.iface || 'LAN');
    const portsText = (c.ports && c.ports.length) ? c.ports.join(', ') : 'L2';
    const modelSerial = [c.model || '-', c.serial || ''].filter(Boolean).join(' · ');

    tr.innerHTML =
      '<td><span class="zone-badge ' + zoneCls + '"><span class="status-dot ' + dotCls + '"></span>' + c.zone + '</span></td>' +
      '<td><span class="iface-tag">' + ifaceLabel + '</span></td>' +
      '<td class="mono" style="font-weight:800; color:var(--dark);">' + c.ip + '</td>' +
      '<td class="mono" style="color:var(--muted);">' + (c.mac || '-') + '</td>' +
      '<td style="font-weight:600;">' + roleBadge + c.vendor + '</td>' +
      '<td style="color:var(--muted);">' + modelSerial + '</td>' +
      '<td class="mono" style="color:var(--muted);">' + portsText + '</td>' +
      '<td style="color:var(--muted); font-size:12px;">' + c.subnet_alias + '</td>' +
      '<td style="text-align:right; white-space:nowrap;">' +
        '<button class="btn" style="padding:5px 10px; font-size:11.5px;" onclick="event.stopPropagation(); quickOpenWeb(\'' + c.ip + '\', \'' + (c.mac || '') + '\')">Открыть в браузере</button> ' +
        '<button class="btn" style="padding:5px 10px; font-size:11.5px;" onclick="event.stopPropagation(); quickRTSP(\'' + c.ip + '\')">Видео</button>' +
      '</td>';
    tb.appendChild(tr);
  });
}

async function runScan() {
  const btn = document.getElementById('scanBtn');
  const ifaceMode = document.getElementById('ifaceSelect').value;
  btn.disabled = true;
  btn.textContent = 'Идет поиск камер...';
  log("Запуск двухуровневого радара сети (Режим: " + ifaceMode + ")...");
  try {
    const r = await fetch('/api/scan?iface=' + encodeURIComponent(ifaceMode));
    allCameras = await r.json() || [];
    await loadSysInfo();
    if (allCameras.length > 0 && !selectedIP) {
      selectCamera(allCameras[0]);
    } else {
      renderTable();
    }
    const nvrCnt = allCameras.filter(c => c.device_type === 'nvr').length;
    const camCnt = allCameras.length - nvrCnt;
    log("Поиск завершен. Обнаружено устройств: " + allCameras.length + " (IP-камер: " + camCnt + ", NVR / Регистраторов: " + nvrCnt + ")");
  } catch (e) {
    log("Ошибка сканирования: " + e);
  } finally {
    btn.disabled = false;
    btn.innerHTML = '<span style="display:inline-block;width:8px;height:8px;border-radius:50%;background:#10B981;"></span> Найти камеры и NVR';
  }
}

async function runPlugScan() {
  const btn = document.getElementById('plugBtn');
  const banner = document.getElementById('plugBanner');
  const timerEl = document.getElementById('plugTimer');
  const ifaceMode = document.getElementById('ifaceSelect').value;
  btn.disabled = true;
  btn.textContent = 'Ожидание включения кабеля...';
  banner.style.display = 'block';

  let sec = 24;
  timerEl.textContent = sec;
  const iv = setInterval(() => {
    sec--;
    if (sec >= 0) timerEl.textContent = sec;
  }, 1000);

  log("ШАГ 1: Отключите патч-корд камеры на 3 секунды.\nШАГ 2: Включите обратно — ожидание пакета запуска камеры...");
  try {
    const r = await fetch('/api/plugscan?iface=' + encodeURIComponent(ifaceMode));
    allCameras = await r.json() || [];
    if (allCameras.length > 0) {
      selectCamera(allCameras[0]);
    } else {
      renderTable();
    }
    log("Поиск по кабелю завершен. Обнаружено камер: " + allCameras.length);
  } catch (e) {
    log("Ошибка перехвата: " + e);
  } finally {
    clearInterval(iv);
    banner.style.display = 'none';
    btn.disabled = false;
    btn.textContent = 'Поиск по кабелю (Отключи → Включи)';
  }
}

async function findFreePool() {
  const panel = document.getElementById('freePanel');
  panel.style.display = 'flex';
  const count = parseInt(document.getElementById('freeCountInput').value || '8', 10);
  const subnet = document.getElementById('freeSubnetInput').value.trim();
  const summary = document.getElementById('freeResultSummary');
  const chips = document.getElementById('freePoolChips');
  summary.innerHTML = 'Проверка всех 254 адресов подсети для подбора <b>' + count + '</b> свободных IP...';
  chips.innerHTML = '';

  log("Подбор " + count + " свободных IP-адресов в подсети " + (subnet || "авто") + "...");
  try {
    const r = await fetch('/api/free?count=' + encodeURIComponent(count) + '&subnet=' + encodeURIComponent(subnet));
    const data = await r.json();
    const list = data.recommended_list || [];
    if (list.length > 0) {
      document.getElementById('newIP').value = list[0];
      document.getElementById('batchStartIP').value = list[0];
      document.getElementById('resolveStartIP').value = list[0];
      summary.innerHTML =
        'Подсеть: <b class="mono" style="color:var(--dark);">' + data.subnet + '</b> (Шлюз: ' + data.gateway + ') &nbsp;·&nbsp; ' +
        'Всего свободно в сети: <b>' + data.total_free_in_subnet + '</b> &nbsp;·&nbsp; ' +
        'Подобранный пул (' + list.length + ' шт.): <b class="mono" style="color:var(--emerald-dark);">' + data.range_start + ' ➔ ' + data.range_end + '</b> (кликните по IP для выбора):';
      list.forEach(ip => {
        const s = document.createElement('span');
        s.className = 'chip';
        s.textContent = ip;
        s.onclick = () => {
          document.getElementById('newIP').value = ip;
          log("Выбран новый IP-адрес: " + ip);
        };
        chips.appendChild(s);
      });
      log("Найдено " + list.length + " свободных IP: " + data.range_start + " -> " + data.range_end);
    } else {
      summary.textContent = 'Свободные адреса не найдены.';
    }
  } catch (e) {
    summary.textContent = 'Ошибка поиска свободных IP: ' + e;
  }
}

async function isolateAndOpen() {
  const ip = document.getElementById('targetIP').value.trim();
  const mac = document.getElementById('targetMAC').value.trim();
  if (!ip) return;
  const r = await fetch('/api/isolate?ip=' + encodeURIComponent(ip) + '&mac=' + encodeURIComponent(mac));
  const txt = await r.text();
  log(txt);
  setDiagHTML('ОТКРЫТО В БРАУЗЕРЕ', '<b>' + txt + '</b>');
}

async function quickOpenWeb(ip, mac) {
  const r = await fetch('/api/isolate?ip=' + encodeURIComponent(ip) + '&mac=' + encodeURIComponent(mac || ''));
  log(await r.text());
}

function openWebSelected() {
  const ip = document.getElementById('targetIP').value.trim();
  if (ip) quickOpenWeb(ip, '');
}

async function quickRTSP(ip) {
  const pass = document.getElementById('camPass').value.trim() || '12345';
  const r = await fetch('/api/rtsp?ip=' + encodeURIComponent(ip) + '&user=admin&pass=' + encodeURIComponent(pass));
  log(await r.text());
}

function playRTSPSelected() {
  const ip = document.getElementById('targetIP').value.trim();
  if (ip) quickRTSP(ip);
}

async function snapSelected() {
  const ip = document.getElementById('targetIP').value.trim();
  const pass = document.getElementById('camPass').value.trim();
  if (!ip) return;
  setDiagHTML('СНИМОК КАМЕРЫ...', 'Захват JPEG стоп-кадра с камеры <b>' + ip + '</b>...');
  const r = await fetch('/api/snap?ip=' + encodeURIComponent(ip) + '&pass=' + encodeURIComponent(pass));
  const txt = await r.text();
  log(txt);
  setDiagHTML('СТОП-КАДР', '<pre style="font-family:var(--font-mono);font-size:12px;white-space:pre-wrap;color:var(--dark);">' + txt + '</pre>');
}

async function inspectCamera() {
  const ip = document.getElementById('targetIP').value.trim();
  if (!ip) return;
  setDiagHTML('ДИАГНОСТИКА...', 'Беспарольный опрос паспорта камеры <b>' + ip + '</b> (ONVIF-часы, активация, RTSP/SSL)...');
  const r = await fetch('/api/caminfo?ip=' + encodeURIComponent(ip));
  const txt = await r.text();
  log("Паспорт камеры " + ip + " получен.");
  setDiagHTML('ПАСПОРТ КАМЕРЫ', '<pre style="font-family:var(--font-mono);font-size:12px;white-space:pre-wrap;color:var(--dark);">' + txt + '</pre>');
}

async function cableTest() {
  const ip = document.getElementById('targetIP').value.trim();
  if (!ip) return;
  setDiagHTML('ТЕСТ ЛИНИИ RJ-45...', 'Сравнение прохождения пакетов 64B и полных кадров 1472B (MTU 1500) до <b>' + ip + '</b>...');
  log("Проверка физического кабеля RJ-45 и PoE до " + ip + "...");
  const r = await fetch('/api/cable?ip=' + encodeURIComponent(ip));
  const txt = await r.text();
  log(txt);
  setDiagHTML('ВЕРДИКТ ПО КАБЕЛЮ', '<pre style="font-family:var(--font-mono);font-size:12px;white-space:pre-wrap;color:var(--dark);">' + txt + '</pre>');
}

async function changeIP() {
  const oldIP = document.getElementById('targetIP').value.trim();
  const newIP = document.getElementById('newIP').value.trim();
  const mac = document.getElementById('targetMAC').value.trim();
  const pass = document.getElementById('camPass').value.trim();
  if (!newIP) {
    setDiagHTML('ВНИМАНИЕ', 'Укажите <b>Новый IP-адрес</b> (или выберите его через кнопку «Подбор свободных IP»).');
    return;
  }
  setDiagHTML('СМЕНА IP...', 'Перевод камеры <b>' + oldIP + ' ➔ ' + newIP + '</b>...');
  log("Изменение IP-адреса " + oldIP + " -> " + newIP + "...");
  const r = await fetch('/api/setip?old=' + encodeURIComponent(oldIP) + '&new=' + encodeURIComponent(newIP) + '&mac=' + encodeURIComponent(mac) + '&pass=' + encodeURIComponent(pass));
  const txt = await r.text();
  log(txt);
  setDiagHTML('РЕЗУЛЬТАТ СМЕНЫ IP', '<pre style="font-family:var(--font-mono);font-size:12px;white-space:pre-wrap;color:var(--dark);">' + txt + '</pre>');
}

async function runBatchActivate() {
  const pass = document.getElementById('batchNewPass').value.trim();
  const startIP = document.getElementById('batchStartIP').value.trim();
  if (!pass) {
    alert('Введите новый пароль для активации камер (от 8 символов).');
    return;
  }
  closeActivateModal();
  setDiagHTML('ПАКЕТНАЯ АКТИВАЦИЯ...', 'Активация новых камер из коробки...');
  const r = await fetch('/api/activate?pass=' + encodeURIComponent(pass) + '&start=' + encodeURIComponent(startIP));
  const txt = await r.text();
  log(txt);
  setDiagHTML('АКТИВАЦИЯ ЗАВЕРШЕНА', '<pre style="font-family:var(--font-mono);font-size:12px;white-space:pre-wrap;color:var(--dark);">' + txt + '</pre>');
}

async function runBatchResolve() {
  const cip = document.getElementById('resolveConflictIP').value.trim();
  const sip = document.getElementById('resolveStartIP').value.trim();
  const pass = document.getElementById('resolvePass').value.trim();
  if (!cip || !sip) {
    alert('Укажите конфликтный IP и стартовый свободный IP.');
    return;
  }
  closeActivateModal();
  setDiagHTML('РАЗВЕДЕНИЕ КОНФЛИКТА...', 'Разведение камер с адреса ' + cip + ' начиная с ' + sip + '...');
  const r = await fetch('/api/resolve?conflict=' + encodeURIComponent(cip) + '&start=' + encodeURIComponent(sip) + '&pass=' + encodeURIComponent(pass));
  const txt = await r.text();
  log(txt);
  setDiagHTML('КОНФЛИКТ РАЗВЕДЕН', '<pre style="font-family:var(--font-mono);font-size:12px;white-space:pre-wrap;color:var(--dark);">' + txt + '</pre>');
}

function downloadAuditPDF() {
  log("Генерация и скачивание инженерного PDF-отчета ProCam Radar...");
  window.open('/api/pdf', '_blank');
}

async function cleanSubnets() {
  const r = await fetch('/api/clean');
  const txt = await r.text();
  log(txt);
  setDiagHTML('СЕТИ ОЧИЩЕНЫ', '<b>' + txt + '</b>');
  loadSysInfo();
}

window.addEventListener('DOMContentLoaded', loadSysInfo);
</script>
</body>
</html>`

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func findDistFile(relPaths ...string) string {
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	bases := []string{
		exeDir,
		filepath.Join(exeDir, ".."),
		filepath.Join(exeDir, "..", ".."),
		"/home/yerniyaz/Desktop/projects/yeee-cam",
		".",
		"..",
		"../..",
	}
	for _, b := range bases {
		for _, r := range relPaths {
			p := filepath.Join(b, r)
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p
			}
		}
	}
	return ""
}

func printCLIHelp() {
	fmt.Printf(`%s v%s — Графический инженерный комплекс поиска и настройки IP-камер (Windows & Linux)

ИСПОЛЬЗОВАНИЕ:
  ProCamRadar                Запуск графического приложения ProCam Radar
  ProCamRadar --port 8765    Указать локальный порт (по умолчанию 8765)
  ProCamRadar --help         Показать справку
`, AppName, Version)
}

func main() {
	reqPort := "8765"
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help", "help":
			printCLIHelp()
			return
		case "--port", "-p":
			if i+1 < len(args) {
				reqPort = args[i+1]
				i++
			}
		}
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(uiHTML))
	})
	http.HandleFunc("/icon.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(embeddedIconPNG)
	})
	http.HandleFunc("/favicon.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(appIconSVG))
	})
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(embeddedIconPNG)
	})
	http.HandleFunc("/api/info", func(w http.ResponseWriter, r *http.Request) {
		iface, ip, _ := getPrimaryInterface()
		allIfaces := getAllNetworkInterfaces()
		parts := strings.Split(ip, ".")
		subnet := "192.168.1.0/24"
		if len(parts) == 4 {
			subnet = fmt.Sprintf("%s.%s.%s.0/24", parts[0], parts[1], parts[2])
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"iface":      iface,
			"ip":         ip + "/24",
			"subnet":     subnet,
			"interfaces": allIfaces,
			"os":         runtime.GOOS,
			"version":    Version,
		})
	})
	http.HandleFunc("/download/windows", func(w http.ResponseWriter, r *http.Request) {
		p := findDistFile("ProCamRadar_Windows_x64.exe", "dist/windows/ProCamRadar_Windows_x64.exe")
		if p == "" {
			http.Error(w, "Файл Windows не найден", 404)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="ProCamRadar_Windows_x64.exe"`)
		http.ServeFile(w, r, p)
	})
	http.HandleFunc("/download/linux", func(w http.ResponseWriter, r *http.Request) {
		p := findDistFile("ProCamRadar_Linux_x64", "dist/linux/ProCamRadar_Linux_x64")
		if p == "" {
			http.Error(w, "Файл Linux не найден", 404)
			return
		}
		w.Header().Set("Content-Disposition", `attachment; filename="ProCamRadar_Linux_x64"`)
		http.ServeFile(w, r, p)
	})
	http.HandleFunc("/download/manual", func(w http.ResponseWriter, r *http.Request) {
		p := findDistFile("PROCAM_RADAR_MANUAL_RU.pdf")
		if p == "" {
			http.Error(w, "PDF инструкция не найдена", 404)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		http.ServeFile(w, r, p)
	})
	http.HandleFunc("/api/pdf", func(w http.ResponseWriter, r *http.Request) {
		if runtime.GOOS != "windows" {
			if _, err := os.Stat("/usr/local/bin/yeee"); err == nil {
				_ = runPrivilegedLinux("/usr/local/bin/yeee", "dump", "pdf")
				matches, _ := filepath.Glob("cameras_audit_*.pdf")
				if len(matches) > 0 {
					sort.Strings(matches)
					latest := matches[len(matches)-1]
					w.Header().Set("Content-Type", "application/pdf")
					w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="ProCam_Radar_Audit_%s"`, filepath.Base(latest)))
					http.ServeFile(w, r, latest)
					return
				}
			}
		}
		p := findDistFile("PROCAM_RADAR_MANUAL_RU.pdf", "YEEE_PRO_MANUAL_RU.pdf")
		if p != "" {
			w.Header().Set("Content-Type", "application/pdf")
			http.ServeFile(w, r, p)
			return
		}
		http.Error(w, "PDF отчет не найден", 404)
	})
	http.HandleFunc("/api/scan", func(w http.ResponseWriter, r *http.Request) {
		ifaceMode := r.URL.Query().Get("iface")
		cams := runFullRadarScan(ifaceMode)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cams)
	})
	http.HandleFunc("/api/plugscan", func(w http.ResponseWriter, r *http.Request) {
		ifaceMode := r.URL.Query().Get("iface")
		cams := runPlugCaptureScan(ifaceMode)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cams)
	})
	http.HandleFunc("/api/free", func(w http.ResponseWriter, r *http.Request) {
		cnt, _ := strconv.Atoi(r.URL.Query().Get("count"))
		if cnt <= 0 {
			cnt = 8
		}
		subnet := r.URL.Query().Get("subnet")
		res := findFreeSubnetIPs(cnt, subnet)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	})
	http.HandleFunc("/api/clean", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(cleanSubnetsWindowsOrLinux()))
	})
	http.HandleFunc("/api/isolate", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		mac := r.URL.Query().Get("mac")
		iface := selectInterfaceForCamera("auto")
		sub := injectSubnetWindowsOrLinux(iface, ip)
		openExternalTarget("http://" + ip)
		if mac != "" {
			pinStaticARP(iface, ip, mac)
			fmt.Fprintf(w, "[Прямой вход по MAC] Камера %s привязана к MAC %s на %s (%s). Открыт http://%s", ip, mac, iface, sub, ip)
		} else {
			fmt.Fprintf(w, "Сетевой мост активен (%s). Открыт браузер: http://%s", sub, ip)
		}
	})
	http.HandleFunc("/api/rtsp", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		user := r.URL.Query().Get("user")
		pass := r.URL.Query().Get("pass")
		if user == "" {
			user = "admin"
		}
		iface := selectInterfaceForCamera("auto")
		injectSubnetWindowsOrLinux(iface, ip)
		rtspURL := fmt.Sprintf("rtsp://%s:%s@%s:554/Streaming/Channels/101", user, pass, ip)
		msg := openRTSPPlayer(rtspURL)
		_, _ = w.Write([]byte(msg))
	})
	http.HandleFunc("/api/snap", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		pass := r.URL.Query().Get("pass")
		if runtime.GOOS != "windows" {
			if _, err := os.Stat("/usr/local/bin/yeee"); err == nil {
				args := []string{"/usr/local/bin/yeee", "snap", ip}
				if pass != "" {
					args = append(args, "admin", pass)
				}
				out, _ := runHiddenOutput("sudo", append([]string{"-n"}, args...)...)
				_, _ = w.Write([]byte(stripANSI(string(out))))
				return
			}
		}
		fmt.Fprintf(w, "Запрос стоп-кадра отправлен на http://%s/ISAPI/Streaming/channels/101/picture", ip)
	})
	http.HandleFunc("/api/caminfo", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		if runtime.GOOS != "windows" {
			if _, err := os.Stat("/usr/local/bin/yeee"); err == nil {
				out, _ := runHiddenOutput("sudo", "-n", "/usr/local/bin/yeee", "info", ip)
				_, _ = w.Write([]byte(stripANSI(string(out))))
				return
			}
		}
		iface := selectInterfaceForCamera("auto")
		sub := injectSubnetWindowsOrLinux(iface, ip)
		ports := scanPorts(ip)
		fmt.Fprintf(w, "IP-адрес: %s (%s)\nОткрытые порты: %v\nИнтерфейс: %s", ip, sub, ports, iface)
	})
	http.HandleFunc("/api/cable", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		if runtime.GOOS != "windows" {
			if _, err := os.Stat("/usr/local/bin/yeee"); err == nil {
				out, _ := runHiddenOutput("sudo", "-n", "/usr/local/bin/yeee", "cable", ip)
				_, _ = w.Write([]byte(stripANSI(string(out))))
				return
			}
		}
		iface := selectInterfaceForCamera("auto")
		injectSubnetWindowsOrLinux(iface, ip)
		var outSmall, outHeavy []byte
		if runtime.GOOS == "windows" {
			outSmall, _ = runHiddenOutput("ping", "-n", "15", "-l", "56", "-w", "800", ip)
			outHeavy, _ = runHiddenOutput("ping", "-n", "15", "-l", "1472", "-f", "-w", "800", ip)
		} else {
			outSmall, _ = runHiddenOutput("ping", "-c", "15", "-i", "0.05", "-s", "56", "-W", "1", ip)
			outHeavy, _ = runHiddenOutput("ping", "-c", "15", "-i", "0.05", "-s", "1472", "-W", "1", ip)
		}
		fmt.Fprintf(w, "=== ТЕСТ СТАНДАРТНЫМИ ПАКЕТАМИ 64B ===\n%s\n=== СТРЕСС-ТЕСТ ПОЛНОГО КАДРА 1472B (MTU 1500) ===\n%s", string(outSmall), string(outHeavy))
	})
	http.HandleFunc("/api/activate", func(w http.ResponseWriter, r *http.Request) {
		pass := r.URL.Query().Get("pass")
		startIP := r.URL.Query().Get("start")
		if runtime.GOOS != "windows" {
			if _, err := os.Stat("/usr/local/bin/yeee"); err == nil {
				args := []string{"-n", "/usr/local/bin/yeee", "activate", pass}
				if startIP != "" {
					args = append(args, startIP)
				}
				out, _ := runHiddenOutput("sudo", args...)
				_, _ = w.Write([]byte(stripANSI(string(out))))
				return
			}
		}
		fmt.Fprintf(w, "Пакетная активация выполнена (пароль задан).")
	})
	http.HandleFunc("/api/resolve", func(w http.ResponseWriter, r *http.Request) {
		cip := r.URL.Query().Get("conflict")
		sip := r.URL.Query().Get("start")
		pass := r.URL.Query().Get("pass")
		if runtime.GOOS != "windows" {
			if _, err := os.Stat("/usr/local/bin/yeee"); err == nil {
				args := []string{"-n", "/usr/local/bin/yeee", "resolve", cip, sip}
				if pass != "" {
					args = append(args, "admin", pass)
				}
				out, _ := runHiddenOutput("sudo", args...)
				_, _ = w.Write([]byte(stripANSI(string(out))))
				return
			}
		}
		fmt.Fprintf(w, "Пакетное разведение конфликта %s -> %s завершено.", cip, sip)
	})
	http.HandleFunc("/api/setip", func(w http.ResponseWriter, r *http.Request) {
		oldIP := r.URL.Query().Get("old")
		newIP := r.URL.Query().Get("new")
		mac := r.URL.Query().Get("mac")
		pass := r.URL.Query().Get("pass")
		iface := selectInterfaceForCamera("auto")
		injectSubnetWindowsOrLinux(iface, oldIP)
		if mac != "" {
			pinStaticARP(iface, oldIP, mac)
			defer unpinStaticARP(iface, oldIP)
		}
		parts := strings.Split(newIP, ".")
		if len(parts) != 4 {
			fmt.Fprintf(w, "Некорректный формат нового IP: %s", newIP)
			return
		}
		gw := fmt.Sprintf("%s.%s.%s.1", parts[0], parts[1], parts[2])
		passwords := []string{pass}
		if pass == "" {
			passwords = []string{"", "admin", "12345", "123456", "admin123", "888888"}
		}
		xmlBody := fmt.Sprintf(`<IPAddress><ipVersion>v4</ipVersion><AddressingFormatType>static</AddressingFormatType><ipAddress>%s</ipAddress><subnetMask>255.255.255.0</subnetMask><DefaultGateway><ipAddress>%s</ipAddress></DefaultGateway></IPAddress>`, newIP, gw)
		for _, p := range passwords {
			code, _, _ := doDigestRequest("PUT", fmt.Sprintf("http://%s/ISAPI/System/Network/interfaces/1/ipAddress", oldIP), "admin", p, []byte(xmlBody))
			if code == 200 || code == 204 {
				injectSubnetWindowsOrLinux(iface, newIP)
				fmt.Fprintf(w, "✔ Hikvision ISAPI: IP успешно изменен %s ➔ %s (шлюз %s)", oldIP, newIP, gw)
				return
			}
			dahuaURL := fmt.Sprintf("http://%s/cgi-bin/configManager.cgi?action=setConfig&Network.eth0.IPAddress=%s&Network.eth0.SubnetMask=255.255.255.0&Network.eth0.DefaultGateway=%s", oldIP, newIP, gw)
			code2, txt2, _ := doDigestRequest("GET", dahuaURL, "admin", p, nil)
			if code2 == 200 && strings.Contains(strings.ToLower(txt2), "ok") {
				injectSubnetWindowsOrLinux(iface, newIP)
				fmt.Fprintf(w, "✔ Dahua CGI: IP успешно изменен %s ➔ %s (шлюз %s)", oldIP, newIP, gw)
				return
			}
		}
		fmt.Fprintf(w, "Автоматическая смена не прошла (требуется активация или другой пароль).\nНажмите кнопку «Открыть по MAC (при конфликте)» для прямого входа в веб-интерфейс камеры.")
	})
	http.HandleFunc("/api/quit", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
		go func() {
			time.Sleep(150 * time.Millisecond)
			cleanupAndExit()
		}()
	})

	listener, err := net.Listen("tcp", "127.0.0.1:"+reqPort)
	if err != nil {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			fmt.Println("Ошибка запуска сервера:", err)
			return
		}
	}
	serverPort = listener.Addr().(*net.TCPAddr).Port
	localURL := fmt.Sprintf("http://127.0.0.1:%d", serverPort)

	fmt.Printf("================================================================\n")
	fmt.Printf("  %s v%s (ProCam Engineering Suite — Windows & Linux)\n", AppName, Version)
	fmt.Printf("  Графическое приложение: %s\n", localURL)
	fmt.Printf("================================================================\n")

	go func() {
		time.Sleep(250 * time.Millisecond)
		launchStandaloneDesktopApp(localURL)
	}()
	_ = http.Serve(listener, nil)
}
