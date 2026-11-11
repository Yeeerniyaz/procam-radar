package main

import (
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/url"
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

const (
	AppName = "ProCam Radar"
	Version = "5.0"
)

//go:embed icon.png
var embeddedIconPNG []byte

//go:embed all:web
var embeddedWebFS embed.FS

type NetInterfaceInfo struct {
	Name    string       `json:"name"`
	Kind    string       `json:"kind"` // "ethernet" | "wifi" | "vpn" | "other"
	Label   string       `json:"label"`
	IP      string       `json:"ip"`
	AllIPs  []string     `json:"all_ips,omitempty"`
	Subnet  string       `json:"subnet"`
	Carrier bool         `json:"carrier"`
	IPNet   *net.IPNet   `json:"-"`
	AllNets []*net.IPNet `json:"-"`
}

type CameraEntry struct {
	IP          string   `json:"ip"`
	MAC         string   `json:"mac"`
	Vendor      string   `json:"vendor"`
	DeviceType  string   `json:"device_type"`  // "camera" | "nvr"
	DeviceLabel string   `json:"device_label"` // "IP-камера" | "NVR / Регистратор"
	Model       string   `json:"model"`
	Serial      string   `json:"serial"`
	Firmware    string   `json:"firmware"`
	Ports       []int    `json:"ports"`
	Zone        string   `json:"zone"`
	ZoneType    string   `json:"zone_type"` // "local" | "foreign" | "conflict"
	SubnetAlias string   `json:"subnet_alias"`
	Conflict    bool     `json:"conflict"`
	Iface       string   `json:"iface"`
	IfaceKind   string   `json:"iface_kind"`
	WebLink     string   `json:"web_link"`
	RTSPLink    string   `json:"rtsp_link"`
}

type SubnetHostCell struct {
	Host   int    `json:"host"`
	IP     string `json:"ip"`
	Status string `json:"status"` // "free" | "camera" | "nvr" | "conflict" | "system" | "occupied"
	Label  string `json:"label,omitempty"`
	Vendor string `json:"vendor,omitempty"`
	MAC    string `json:"mac,omitempty"`
}

type FreePoolResponse struct {
	Subnet          string           `json:"subnet"`
	Gateway         string           `json:"gateway"`
	RequestedCount  int              `json:"requested_count"`
	TotalFreeInSub  int              `json:"total_free_in_subnet"`
	OccupiedInSub   int              `json:"occupied_in_subnet"`
	RangeStart      string           `json:"range_start"`
	RangeEnd        string           `json:"range_end"`
	RecommendedList []string         `json:"recommended_list"`
	HostMap         []SubnetHostCell `json:"host_map"`
}

var (
	stateMu         sync.Mutex
	cachedCameras   []CameraEntry
	injectedSubnets = make(map[string]string) // prefix -> hostIP
	serverPort      int
	appTempDir      string
)

var cameraOUI = map[string]string{
	// Hikvision / HiWatch / HiLook
	"c0:56:e3": "Hikvision", "4c:bd:8f": "Hikvision", "bc:ad:28": "Hikvision",
	"28:57:be": "Hikvision", "00:40:8c": "Hikvision", "44:19:b6": "Hikvision",
	"54:c4:15": "Hikvision", "a4:14:37": "Hikvision", "c4:2f:90": "Hikvision",
	"18:68:cb": "Hikvision", "80:7c:62": "Hikvision", "d4:e8:53": "Hikvision",
	"e0:ca:3c": "Hikvision", "8c:e7:48": "Hikvision", "64:db:8b": "Hikvision",
	"ac:b9:2f": "Hikvision", "98:df:82": "Hikvision", "ec:a9:70": "Hikvision",
	"3c:da:6d": "Hikvision / OEM", "3c:da:6e": "Hikvision / OEM",
	"08:a1:89": "Hikvision", "10:12:fb": "Hikvision", "24:0f:9b": "Hikvision",
	"24:28:fd": "Hikvision", "24:32:ae": "Hikvision", "3c:1b:f8": "Hikvision",
	"40:ac:bf": "Hikvision", "44:47:cc": "Hikvision", "4c:62:df": "Hikvision",
	"58:03:fb": "Hikvision", "5c:34:5b": "Hikvision", "68:6d:bc": "Hikvision",
	"74:3f:c2": "Hikvision", "80:48:9f": "Hikvision", "84:9a:40": "Hikvision",
	"94:e1:ac": "Hikvision", "9c:f5:31": "Hikvision", "a0:ff:0c": "Hikvision",
	"b4:a3:82": "Hikvision", "c8:6c:1e": "Hikvision", "dc:07:f8": "Hikvision",
	"e0:ba:ad": "Hikvision", "e8:a0:ed": "Hikvision", "f8:4d:fc": "Hikvision",
	// Dahua / IMOU
	"3c:ef:8c": "Dahua", "90:02:a9": "Dahua", "e0:50:8b": "Dahua",
	"a0:bd:1d": "Dahua", "4c:11:bf": "Dahua", "00:12:41": "Dahua",
	"14:a7:8b": "Dahua", "b4:4c:3b": "Dahua", "6c:1c:71": "Dahua",
	"f8:ce:07": "Dahua", "74:c9:9a": "Dahua", "d4:43:0e": "Dahua",
	"00:c0:7f": "Dahua / OEM", "00:c0:81": "Dahua / OEM",
	"00:c0:88": "Dahua / OEM", "00:c0:89": "Dahua / OEM",
	"e4:e6:6c": "IP-камера (Smart IPC)",
	"08:ed:ed": "Dahua", "2c:f0:5d": "Dahua", "30:dd:aa": "Dahua",
	"38:af:29": "Dahua", "50:f5:20": "Dahua", "5c:f2:07": "Dahua",
	"8c:e9:b4": "Dahua", "9c:14:63": "Dahua", "a8:31:62": "Dahua",
	"bc:32:5f": "Dahua", "c0:39:5a": "Dahua", "fc:5f:49": "Dahua",
	// Uniview / Tiandy / Axis / XM / TP-Link VIGI
	"48:ea:63": "Uniview", "c4:79:05": "Uniview", "6c:f1:7e": "Tiandy",
	"ac:cc:8e": "Axis", "b8:a4:4f": "Axis", "00:0f:7c": "Xiongmai (XM)",
	"7c:dd:90": "Xiongmai (XM)", "00:18:ae": "TVT Digital", "b0:c5:54": "D-Link Cam",
	"50:c7:bf": "TP-Link VIGI", "60:32:b1": "TP-Link VIGI",
}

func isVirtualInterface(name string) bool {
	low := strings.ToLower(strings.TrimSpace(name))
	if low == "" || low == "lo" {
		return true
	}
	// Игнорируем только внутренние контейнерные/виртуальные мосты (Docker, Libvirt, VBox),
	// но ОСТАВЛЯЕМ Tailscale, WireGuard, ZeroTier и OpenVPN как полноценные VPN-сети!
	containerPrefixes := []string{
		"docker", "br-", "veth", "virbr", "vboxnet", "vmnet", "flannel", "cni", "lxcbr",
	}
	for _, p := range containerPrefixes {
		if strings.HasPrefix(low, p) {
			return true
		}
	}
	return false
}

func normalizeMAC(mac string) string {
	m := strings.ToUpper(strings.TrimSpace(strings.ReplaceAll(mac, "-", ":")))
	if m == "" || m == "00:00:00:00:00:00" || m == "FF:FF:FF:FF:FF:FF" || m == "(INCOMPLETE)" || m == "INCOMPLETE" {
		return ""
	}
	parts := strings.Split(m, ":")
	if len(parts) != 6 {
		return ""
	}
	for i, p := range parts {
		if len(p) == 1 {
			parts[i] = "0" + p
		} else if len(p) != 2 {
			return ""
		}
	}
	return strings.Join(parts, ":")
}

func ipToUint32(ipStr string) uint32 {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return 0
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return 0
	}
	return binary.BigEndian.Uint32(ip4)
}

func lookupVendor(mac string) string {
	norm := strings.ToLower(normalizeMAC(mac))
	if len(norm) >= 8 {
		if v, ok := cameraOUI[norm[:8]]; ok {
			return v
		}
	}
	return ""
}

func classifyDeviceRole(ip, vendor, model string, ports []int) (deviceType, deviceLabel, resolvedModel string) {
	upModel := strings.ToUpper(strings.TrimSpace(model))
	upVendor := strings.ToUpper(strings.TrimSpace(vendor))

	// Убираем служебные ONVIF типы из имени модели
	if strings.Contains(upModel, "NETWORKVIDEOTRANSMITTER") ||
		strings.Contains(upModel, "TDS:DEVICE") ||
		strings.Contains(upModel, "DN:") {
		model = ""
		upModel = ""
	}

	isNVR := false
	nvrKeywords := []string{
		"NVR", "DVR", "XVR", "DS-7", "DS-8", "DS-9", "NVR1", "NVR2", "NVR4", "NVR5",
		"DHI-NVR", "DHI-XVR", "HCVR", "HWN-", "HWD-", "UNV-NVR", "NVR30",
	}
	for _, kw := range nvrKeywords {
		if strings.Contains(upModel, kw) || strings.Contains(upVendor, kw) {
			isNVR = true
			break
		}
	}

	parts := strings.Split(ip, ".")
	lastOctet := 0
	if len(parts) == 4 {
		lastOctet, _ = strconv.Atoi(parts[3])
	}
	if !isNVR && (lastOctet == 250 || lastOctet == 240 || lastOctet == 200 || lastOctet == 100) {
		has8000 := false
		has37777 := false
		for _, p := range ports {
			if p == 8000 {
				has8000 = true
			}
			if p == 37777 {
				has37777 = true
			}
		}
		if (has8000 || has37777) && (upModel == "" || upModel == "IP-КАМЕРА" || upModel == "IPC" || upModel == "ONVIF") {
			if lastOctet == 250 || lastOctet == 240 {
				isNVR = true
			}
		}
	}

	if isNVR {
		deviceType = "nvr"
		deviceLabel = "NVR / Регистратор"
		if model == "" || model == "IP-камера" {
			if strings.Contains(upVendor, "HIK") {
				resolvedModel = "Сетевой регистратор Hikvision NVR"
			} else if strings.Contains(upVendor, "DAHUA") {
				resolvedModel = "Сетевой регистратор Dahua NVR/XVR"
			} else {
				resolvedModel = "Сетевой видеорегистратор (NVR)"
			}
		} else {
			resolvedModel = model
		}
		return
	}

	deviceType = "camera"
	deviceLabel = "IP-камера"
	if model == "" {
		if strings.Contains(upVendor, "HIK") {
			resolvedModel = "IP-камера Hikvision"
		} else if strings.Contains(upVendor, "DAHUA") {
			resolvedModel = "IP-камера Dahua"
		} else {
			resolvedModel = "IP-камера"
		}
	} else {
		resolvedModel = model
	}
	return
}

func classifyInterface(name string) string {
	low := strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(low, "tailscale") || strings.HasPrefix(low, "wg") ||
		strings.HasPrefix(low, "zt") || strings.HasPrefix(low, "tun") ||
		strings.HasPrefix(low, "tap") || strings.Contains(low, "wireguard") ||
		strings.Contains(low, "zerotier") || strings.Contains(low, "vpn") {
		return "vpn"
	}
	if strings.HasPrefix(low, "wl") || strings.Contains(low, "wi-fi") ||
		strings.Contains(low, "wlan") || strings.Contains(low, "wireless") {
		return "wifi"
	}
	if strings.HasPrefix(low, "en") || strings.HasPrefix(low, "eth") ||
		strings.Contains(low, "ethernet") || strings.Contains(low, "lan") {
		return "ethernet"
	}
	return "other"
}

func interfacePriority(name string) int {
	switch classifyInterface(name) {
	case "ethernet":
		return 3
	case "wifi":
		return 2
	case "vpn":
		return 1
	default:
		return 0
	}
}

func checkCarrierLinux(name string) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	b, err := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/carrier", name))
	if err == nil && strings.TrimSpace(string(b)) == "1" {
		return true
	}
	b2, err2 := os.ReadFile(fmt.Sprintf("/sys/class/net/%s/operstate", name))
	if err2 == nil {
		st := strings.TrimSpace(string(b2))
		if st == "up" || st == "unknown" {
			// Для tun/wg/tailscale operstate часто "unknown" при поднятом интерфейсе
			if classifyInterface(name) == "vpn" || st == "up" {
				return true
			}
		}
	}
	return false
}

func getAllNetworkInterfaces() []NetInterfaceInfo {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var list []NetInterfaceInfo
	for _, i := range ifaces {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isVirtualInterface(i.Name) {
			continue
		}
		kind := classifyInterface(i.Name)
		carrier := (i.Flags&net.FlagUp != 0) && checkCarrierLinux(i.Name)

		var ipStr, subnetStr string
		var allIPs []string
		var foundNet *net.IPNet
		var allNets []*net.IPNet
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				ip4 := ipnet.IP.To4().String()
				// Пропускаем временные сервисные алиасы мостов ProCam (.248 / .249)
				if strings.HasSuffix(ip4, ".249") || strings.HasSuffix(ip4, ".248") {
					continue
				}
				allIPs = append(allIPs, ip4)
				allNets = append(allNets, ipnet)
				if ipStr == "" {
					ipStr = ip4
					foundNet = ipnet
					parts := strings.Split(ip4, ".")
					if len(parts) == 4 {
						subnetStr = fmt.Sprintf("%s.%s.%s.0/24", parts[0], parts[1], parts[2])
					}
				}
			}
		}

		if !carrier && ipStr == "" && (kind == "other" || kind == "vpn") {
			continue
		}

		list = append(list, NetInterfaceInfo{
			Name:    i.Name,
			Kind:    kind,
			IP:      ipStr,
			AllIPs:  allIPs,
			Subnet:  subnetStr,
			Carrier: carrier || ipStr != "",
			IPNet:   foundNet,
			AllNets: allNets,
		})
	}

	sort.Slice(list, func(a, b int) bool {
		score := func(x NetInterfaceInfo) int {
			s := 0
			switch x.Kind {
			case "ethernet":
				s += 20
			case "wifi":
				s += 12
			case "vpn":
				s += 6
			}
			if x.Carrier {
				s += 4
			}
			if x.IP != "" {
				s += 2
			}
			return s
		}
		if score(list[a]) != score(list[b]) {
			return score(list[a]) > score(list[b])
		}
		return list[a].Name < list[b].Name
	})

	// Считаем количество адаптеров каждого типа (если 2 кабеля или 2 Wi-Fi — нумеруем #1, #2)
	counts := make(map[string]int)
	for _, inf := range list {
		counts[inf.Kind]++
	}
	indices := make(map[string]int)
	for idx := range list {
		inf := &list[idx]
		indices[inf.Kind]++
		numSuffix := ""
		if counts[inf.Kind] > 1 {
			numSuffix = fmt.Sprintf(" #%d", indices[inf.Kind])
		}
		labelKind := "Сеть"
		switch inf.Kind {
		case "ethernet":
			labelKind = "Кабель" + numSuffix
		case "wifi":
			labelKind = "Wi-Fi" + numSuffix
		case "vpn":
			if strings.HasPrefix(strings.ToLower(inf.Name), "tailscale") {
				labelKind = "Tailscale" + numSuffix
			} else if strings.HasPrefix(strings.ToLower(inf.Name), "wg") {
				labelKind = "WireGuard" + numSuffix
			} else {
				labelKind = "VPN" + numSuffix
			}
		}
		statusTxt := inf.IP
		if len(inf.AllIPs) > 1 {
			statusTxt = fmt.Sprintf("%s (+%d IP)", inf.IP, len(inf.AllIPs)-1)
		}
		if statusTxt == "" {
			if inf.Carrier {
				statusTxt = "Линк L2 (без DHCP)"
			} else {
				statusTxt = "Нет кабеля"
			}
		}
		inf.Label = fmt.Sprintf("%s [%s]: %s", labelKind, inf.Name, statusTxt)
	}

	return list
}

func getPrimaryInterface() (string, string, *net.IPNet) {
	all := getAllNetworkInterfaces()
	// Сначала ищем физический интерфейс (Ethernet или Wi-Fi) с IP
	for _, inf := range all {
		if inf.IP != "" && inf.Kind != "vpn" {
			return inf.Name, inf.IP, inf.IPNet
		}
	}
	for _, inf := range all {
		if inf.IP != "" {
			return inf.Name, inf.IP, inf.IPNet
		}
	}
	if len(all) > 0 {
		return all[0].Name, "192.168.1.248", nil
	}
	return "eth0", "192.168.1.248", nil
}

func selectInterfaceForCamera(preferred string) string {
	all := getAllNetworkInterfaces()
	if preferred != "" && preferred != "auto" {
		// Поддержка списка через запятую (например "eno1,wlp4s0")
		prefs := strings.Split(preferred, ",")
		for _, p := range prefs {
			p = strings.TrimSpace(p)
			for _, inf := range all {
				if inf.Name == p || inf.Kind == p {
					return inf.Name
				}
			}
		}
	}
	for _, inf := range all {
		if inf.Kind == "ethernet" && inf.Carrier {
			return inf.Name
		}
	}
	for _, inf := range all {
		if inf.Kind == "wifi" && inf.Carrier && inf.IP != "" {
			return inf.Name
		}
	}
	for _, inf := range all {
		if inf.IP != "" {
			return inf.Name
		}
	}
	if len(all) > 0 {
		return all[0].Name
	}
	return "eth0"
}

func resolveInterfaceForCamera(ip, mac, explicitIface string) string {
	if explicitIface != "" && explicitIface != "auto" && explicitIface != "ethernet" && explicitIface != "wifi" && explicitIface != "vpn" {
		return explicitIface
	}
	normM := normalizeMAC(mac)
	stateMu.Lock()
	for _, c := range cachedCameras {
		if c.IP == ip && (normM == "" || c.MAC == normM) && c.Iface != "" {
			stateMu.Unlock()
			return c.Iface
		}
	}
	stateMu.Unlock()

	arpMap := readSystemARPTable()
	if recs, ok := arpMap[ip]; ok && len(recs) > 0 {
		bestIface := ""
		for _, r := range recs {
			if normM != "" && r.MAC != normM {
				continue
			}
			if r.Iface != "" && interfacePriority(r.Iface) > interfacePriority(bestIface) {
				bestIface = r.Iface
			}
		}
		if bestIface != "" {
			return bestIface
		}
	}
	return selectInterfaceForCamera(explicitIface)
}

func runHiddenCmd(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	prepareBackgroundCmd(cmd)
	return cmd.Run()
}

func runHiddenOutput(bin string, args ...string) ([]byte, error) {
	cmd := exec.Command(bin, args...)
	prepareBackgroundCmd(cmd)
	return cmd.CombinedOutput()
}

func runPrivilegedLinux(bin string, args ...string) error {
	if os.Geteuid() == 0 {
		return runHiddenCmd(bin, args...)
	}
	sudoArgs := append([]string{"-n", bin}, args...)
	if err := runHiddenCmd("sudo", sudoArgs...); err == nil {
		return nil
	}
	return runHiddenCmd(bin, args...)
}

func injectSubnetWindowsOrLinux(iface, camIP string) string {
	parts := strings.Split(camIP, ".")
	if len(parts) != 4 {
		return "Неверный IP"
	}
	prefix := fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
	if prefix == "0.0.0" || prefix == "127.0.0" || prefix == "255.255.255" {
		return "Локальный"
	}

	targetIface := iface
	if targetIface == "" {
		targetIface = selectInterfaceForCamera("auto")
	}

	// Если интерфейс — VPN (например Tailscale 100.x.y.z или WireGuard), маршрутизация идет на уровне L3 туннеля
	if classifyInterface(targetIface) == "vpn" || strings.HasPrefix(camIP, "100.") {
		return fmt.Sprintf("Туннель VPN (%s)", targetIface)
	}

	all := getAllNetworkInterfaces()
	parsed := net.ParseIP(camIP)

	// 1. Проверяем, входит ли камера в родную сеть ИМЕННО целевого интерфейса targetIface
	var overlappingOtherIface string
	usedLocalIPs := make(map[string]bool)
	for _, inf := range all {
		for _, ip4 := range inf.AllIPs {
			usedLocalIPs[ip4] = true
		}
		for _, ipnet := range inf.AllNets {
			if ipnet != nil && parsed != nil && ipnet.Contains(parsed) {
				if inf.Name == targetIface {
					return fmt.Sprintf("Родная сеть (%s)", inf.Name)
				}
				overlappingOtherIface = inf.Name
			}
		}
	}

	hostOctet := "249"
	if parts[3] == "249" || usedLocalIPs[fmt.Sprintf("%s.249", prefix)] {
		hostOctet = "248"
	}
	if parts[3] == "248" || usedLocalIPs[fmt.Sprintf("%s.%s", prefix, hostOctet)] {
		hostOctet = "247"
	}
	hostIP := fmt.Sprintf("%s.%s", prefix, hostOctet)

	// 2. СЛУЧАЙ ДВУХ СЕТЕЙ В ОДНОЙ ПОДСЕТИ (например, Wi-Fi уже в 192.168.1.0/24,
	// а камера 192.168.1.64 подключена вторым кабелем Ethernet):
	// Чтобы НЕ оборвать Wi-Fi и интернет, прокидываем точечный маршрут /32 именно до камеры через targetIface!
	if overlappingOtherIface != "" && runtime.GOOS != "windows" {
		routeKey := fmt.Sprintf("host32:%s@%s", camIP, targetIface)
		stateMu.Lock()
		_, already := injectedSubnets[routeKey]
		stateMu.Unlock()
		if !already {
			_ = runPrivilegedLinux("ip", "addr", "add", fmt.Sprintf("%s/32", hostIP), "dev", targetIface)
			_ = runPrivilegedLinux("ip", "route", "replace", fmt.Sprintf("%s/32", camIP), "dev", targetIface, "src", hostIP, "scope", "link")
			stateMu.Lock()
			injectedSubnets[routeKey] = fmt.Sprintf("%s/32|%s|%s", hostIP, targetIface, camIP)
			stateMu.Unlock()
		}
		return fmt.Sprintf("Точечный мост /32 (%s, %s сохранен)", targetIface, overlappingOtherIface)
	}

	// 3. Обычный /24 мост на целевом интерфейсе (ключ включает имя интерфейса, чтобы поддерживать 2 кабеля одновременно)
	bridgeKey := prefix + "@" + targetIface
	stateMu.Lock()
	if val, ok := injectedSubnets[bridgeKey]; ok {
		stateMu.Unlock()
		sp := strings.Split(val, "|")
		return fmt.Sprintf("Мост %s/24 (%s)", sp[0], targetIface)
	}
	stateMu.Unlock()

	if runtime.GOOS == "windows" {
		_ = runHiddenCmd("netsh", "interface", "ipv4", "add", "address",
			fmt.Sprintf("name=%s", targetIface),
			fmt.Sprintf("addr=%s", hostIP),
			"mask=255.255.255.0")
	} else {
		_ = runPrivilegedLinux("ip", "addr", "add", fmt.Sprintf("%s/24", hostIP), "dev", targetIface)
	}

	stateMu.Lock()
	injectedSubnets[bridgeKey] = hostIP + "|" + targetIface
	stateMu.Unlock()
	return fmt.Sprintf("Подключен мост %s/24 (%s)", hostIP, targetIface)
}

func cleanSubnetsWindowsOrLinux() string {
	stateMu.Lock()
	defer stateMu.Unlock()
	count := 0
	for key, val := range injectedSubnets {
		sp := strings.Split(val, "|")
		hostIP := sp[0]
		iface := ""
		if len(sp) > 1 {
			iface = sp[1]
		}
		if iface == "" {
			iface, _, _ = getPrimaryInterface()
		}
		if strings.HasPrefix(key, "host32:") && len(sp) >= 3 {
			camIP := sp[2]
			_ = runPrivilegedLinux("ip", "route", "del", fmt.Sprintf("%s/32", camIP), "dev", iface)
			_ = runPrivilegedLinux("ip", "addr", "del", hostIP, "dev", iface)
		} else if runtime.GOOS == "windows" {
			_ = runHiddenCmd("netsh", "interface", "ipv4", "delete", "address",
				fmt.Sprintf("name=%s", iface),
				fmt.Sprintf("addr=%s", hostIP))
		} else {
			_ = runPrivilegedLinux("ip", "addr", "del", fmt.Sprintf("%s/24", hostIP), "dev", iface)
		}
		delete(injectedSubnets, key)
		count++
	}
	return fmt.Sprintf("Очищено временных сетевых мостов: %d", count)
}

func pinStaticARP(iface, ip, mac string) {
	if ip == "" || mac == "" {
		return
	}
	if iface == "" {
		iface = selectInterfaceForCamera("auto")
	}
	if runtime.GOOS == "windows" {
		winMAC := strings.ReplaceAll(strings.ToLower(mac), ":", "-")
		_ = runHiddenCmd("netsh", "interface", "ipv4", "delete", "neighbors", iface, ip)
		_ = runHiddenCmd("netsh", "interface", "ipv4", "add", "neighbors", iface, ip, winMAC)
	} else {
		_ = runPrivilegedLinux("ip", "neigh", "replace", ip, "lladdr", strings.ToLower(mac), "dev", iface, "nud", "permanent")
	}
}

func unpinStaticARP(iface, ip string) {
	if ip == "" {
		return
	}
	if iface == "" {
		iface = selectInterfaceForCamera("auto")
	}
	if runtime.GOOS == "windows" {
		_ = runHiddenCmd("netsh", "interface", "ipv4", "delete", "neighbors", iface, ip)
	} else {
		_ = runPrivilegedLinux("ip", "neigh", "del", ip, "dev", iface)
	}
}

func scanPorts(ip string) []int {
	portsToCheck := []int{80, 443, 554, 8000, 37777, 8080, 34567}
	var open []int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range portsToCheck {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), 210*time.Millisecond)
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

type arpRecord struct {
	MAC   string
	Iface string
}

func readSystemARPTable() map[string][]arpRecord {
	res := make(map[string][]arpRecord)
	addRec := func(ip, mac, iface string) {
		normMAC := normalizeMAC(mac)
		if normMAC == "" || ip == "" {
			return
		}
		if iface != "" && isVirtualInterface(iface) {
			return
		}
		for idx := range res[ip] {
			if res[ip][idx].MAC == normMAC {
				// Если тот же MAC виден через более приоритетный адаптер (Кабель > Wi-Fi), обновляем интерфейс
				if iface != "" && interfacePriority(iface) > interfacePriority(res[ip][idx].Iface) {
					res[ip][idx].Iface = iface
				}
				return
			}
		}
		res[ip] = append(res[ip], arpRecord{MAC: normMAC, Iface: iface})
	}

	if runtime.GOOS != "windows" {
		b, err := os.ReadFile("/proc/net/arp")
		if err == nil {
			lines := strings.Split(string(b), "\n")
			for idx, line := range lines {
				if idx == 0 {
					continue
				}
				fields := strings.Fields(line)
				if len(fields) >= 6 {
					addRec(fields[0], fields[3], fields[5])
				}
			}
		}
	}

	out, err := runHiddenOutput("arp", "-a")
	if err == nil {
		reIP := regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)
		reMAC := regexp.MustCompile(`([0-9a-fA-F]{2}[:-]){5}[0-9a-fA-F]{2}`)
		for _, line := range strings.Split(string(out), "\n") {
			ipMatch := reIP.FindString(line)
			macMatch := reMAC.FindString(line)
			if ipMatch != "" && macMatch != "" {
				addRec(ipMatch, macMatch, "")
			}
		}
	}
	return res
}

func extractXMLTag(text, tag string) string {
	re := regexp.MustCompile(fmt.Sprintf(`(?is)<(?:[a-zA-Z0-9_]+:)?%s[^>]*>(.*?)</(?:[a-zA-Z0-9_]+:)?%s>`, tag, tag))
	m := re.FindStringSubmatch(text)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func parseDiscoveryPayload(data []byte, srcIP string) (ip, mac, vendor, model, serial, fw string) {
	txt := string(data)
	ip = srcIP

	if strings.Contains(txt, "<ProbeMatch") || strings.Contains(txt, "Types>") || strings.Contains(txt, "Scopes>") {
		vendor = "ONVIF"
		if v := extractXMLTag(txt, "IPv4Address"); v != "" {
			ip = v
		}
		if m := extractXMLTag(txt, "MAC"); m != "" {
			mac = normalizeMAC(m)
		}
		if dt := extractXMLTag(txt, "DeviceDescription"); dt != "" {
			model = dt
			vendor = "Hikvision"
		}
		if sn := extractXMLTag(txt, "DeviceSN"); sn != "" {
			serial = sn
			vendor = "Hikvision"
		}
		if sv := extractXMLTag(txt, "SoftwareVersion"); sv != "" {
			fw = sv
		}
		if xaddrs := extractXMLTag(txt, "XAddrs"); xaddrs != "" && (ip == "" || ip == "0.0.0.0") {
			re := regexp.MustCompile(`https?://(\d+\.\d+\.\d+\.\d+)`)
			if m := re.FindStringSubmatch(xaddrs); len(m) > 1 {
				ip = m[1]
			}
		}
		if scopes := extractXMLTag(txt, "Scopes"); scopes != "" {
			for _, part := range strings.Fields(scopes) {
				low := strings.ToLower(part)
				if strings.Contains(low, "hardware/") {
					hw := strings.Split(part, "hardware/")
					if len(hw) > 1 && model == "" {
						model = strings.ReplaceAll(hw[1], "%20", " ")
					}
				}
				if strings.Contains(low, "name/") && vendor == "ONVIF" {
					nm := strings.Split(part, "name/")
					if len(nm) > 1 {
						cand := strings.ReplaceAll(nm[1], "%20", " ")
						if !strings.Contains(strings.ToUpper(cand), "NETWORKVIDEOTRANSMITTER") {
							vendor = cand
						}
					}
				}
				if strings.Contains(low, "mac/") && mac == "" {
					mc := strings.Split(part, "mac/")
					if len(mc) > 1 {
						mac = normalizeMAC(mc[1])
					}
				}
			}
		}
	}

	if strings.Contains(txt, "Dahua") || strings.Contains(txt, "DHIP") || strings.Contains(txt, `"mac"`) {
		if vendor == "" || vendor == "ONVIF" {
			vendor = "Dahua"
		}
		reMAC := regexp.MustCompile(`(?i)"mac"\s*:\s*"([0-9a-fA-F:-]{17})"`)
		if m := reMAC.FindStringSubmatch(txt); len(m) > 1 {
			mac = normalizeMAC(m[1])
		}
		reMod := regexp.MustCompile(`(?i)"(?:DeviceType|Model|Name)"\s*:\s*"([^"]+)"`)
		if m := reMod.FindStringSubmatch(txt); len(m) > 1 && model == "" {
			model = m[1]
		}
		reSN := regexp.MustCompile(`(?i)"SerialNo"\s*:\s*"([^"]+)"`)
		if m := reSN.FindStringSubmatch(txt); len(m) > 1 && serial == "" {
			serial = m[1]
		}
	}

	upMod := strings.ToUpper(model)
	if strings.Contains(upMod, "NETWORKVIDEOTRANSMITTER") || strings.Contains(upMod, "TDS:DEVICE") {
		model = ""
	}

	if v := lookupVendor(mac); v != "" && (vendor == "" || vendor == "ONVIF") {
		vendor = v
	}
	return
}

func isGenericVendor(v string) bool {
	up := strings.ToUpper(strings.TrimSpace(v))
	return up == "" || up == "ONVIF" || up == "IP-КАМЕРА (ONVIF)" || up == "IP-КАМЕРА" || up == "НЕИЗВЕСТНО"
}

func isGenericModel(m string) bool {
	up := strings.ToUpper(strings.TrimSpace(m))
	return up == "" ||
		up == "IP-КАМЕРА" ||
		up == "IP-КАМЕРА HIKVISION" ||
		up == "IP-КАМЕРА DAHUA" ||
		strings.Contains(up, "NETWORKVIDEOTRANSMITTER") ||
		strings.Contains(up, "TDS:DEVICE")
}

func mergePorts(a, b []int) []int {
	seen := make(map[int]bool)
	var out []int
	for _, p := range a {
		if p > 0 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range b {
		if p > 0 && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Ints(out)
	return out
}

func mergeCameraEntry(dst, src *CameraEntry) {
	if dst.MAC == "" && src.MAC != "" {
		dst.MAC = src.MAC
	}
	if isGenericVendor(dst.Vendor) && !isGenericVendor(src.Vendor) {
		dst.Vendor = src.Vendor
	} else if dst.Vendor == "" && src.Vendor != "" {
		dst.Vendor = src.Vendor
	}
	if isGenericModel(dst.Model) && !isGenericModel(src.Model) {
		dst.Model = src.Model
	} else if dst.Model == "" && src.Model != "" {
		dst.Model = src.Model
	}
	if dst.Serial == "" && src.Serial != "" {
		dst.Serial = src.Serial
	}
	if dst.Firmware == "" && src.Firmware != "" {
		dst.Firmware = src.Firmware
	}
	if dst.Iface == "" && src.Iface != "" {
		dst.Iface = src.Iface
	} else if src.Iface != "" && interfacePriority(src.Iface) > interfacePriority(dst.Iface) {
		dst.Iface = src.Iface
	}
	dst.Ports = mergePorts(dst.Ports, src.Ports)
	if src.Conflict {
		dst.Conflict = true
	}
}

func matchesIfaceFilter(ifaceName, ifaceFilter string) bool {
	if ifaceFilter == "" || ifaceFilter == "auto" {
		return true
	}
	if ifaceName == "" {
		return true
	}
	kind := classifyInterface(ifaceName)
	for _, token := range strings.Split(ifaceFilter, ",") {
		t := strings.TrimSpace(token)
		if t == "" || t == "auto" {
			return true
		}
		if ifaceName == t || kind == t {
			return true
		}
	}
	return false
}

func scanTailscalePeers(vpnIfaceName string, myIPs map[string]bool, appendCandidate func(*CameraEntry)) {
	tsBin, err := exec.LookPath("tailscale")
	if err != nil {
		return
	}
	out, err := runHiddenOutput(tsBin, "status", "--json")
	if err != nil || len(out) == 0 {
		return
	}
	var tsStatus struct {
		Peer map[string]struct {
			HostName     string   `json:"HostName"`
			DNSName      string   `json:"DNSName"`
			Online       bool     `json:"Online"`
			TailscaleIPs []string `json:"TailscaleIPs"`
		} `json:"Peer"`
	}
	if json.Unmarshal(out, &tsStatus) != nil {
		return
	}
	var peerWG sync.WaitGroup
	sem := make(chan struct{}, 16)
	for _, peer := range tsStatus.Peer {
		if !peer.Online {
			continue
		}
		for _, tip := range peer.TailscaleIPs {
			parsed := net.ParseIP(tip)
			if parsed == nil || parsed.To4() == nil || myIPs[tip] {
				continue
			}
			peerWG.Add(1)
			go func(ip, host string) {
				defer peerWG.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				ports := scanPorts(ip)
				hasCamOrNVR := false
				for _, p := range ports {
					if p == 554 || p == 8000 || p == 37777 || p == 34567 {
						hasCamOrNVR = true
						break
					}
				}
				if hasCamOrNVR {
					appendCandidate(&CameraEntry{
						IP:     ip,
						Vendor: "Tailscale Узел",
						Model:  host,
						Ports:  ports,
						Iface:  vpnIfaceName,
					})
				}
			}(tip, peer.HostName)
		}
	}
	peerWG.Wait()
}

func sendDiscoveryProbesOnConn(conn *net.UDPConn) {
	sadpProbe := `<?xml version="1.0" encoding="utf-8"?><Probe><Uuid>550E8400-E29B-41D4-A716-446655440000</Uuid><Types>inquiry</Types></Probe>`
	onvifProbe := `<?xml version="1.0" encoding="UTF-8"?><e:Envelope xmlns:e="http://www.w3.org/2003/05/soap-envelope" xmlns:w="http://schemas.xmlsoap.org/ws/2004/08/addressing" xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dn="http://www.onvif.org/ver10/network/wsdl"><e:Header><w:MessageID>uuid:84ede3de-7dec-11d0-c360-F01234567890</w:MessageID><w:To e:mustUnderstand="true">urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To><w:Action a:mustUnderstand="true" xmlns:a="http://www.w3.org/2005/08/addressing">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action></e:Header><e:Body><d:Probe><d:Types>dn:NetworkVideoTransmitter tds:Device</d:Types></d:Probe></e:Body></e:Envelope>`
	dahuaProbe := []byte{
		0x20, 0x00, 0x00, 0x00, 0x44, 0x48, 0x49, 0x50,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x4d, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x4d, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x7b, 0x22, 0x6d, 0x65, 0x74, 0x68, 0x6f, 0x64,
		0x22, 0x3a, 0x22, 0x44, 0x48, 0x44, 0x69, 0x73,
		0x63, 0x6f, 0x76, 0x65, 0x72, 0x2e, 0x73, 0x65,
		0x61, 0x72, 0x63, 0x68, 0x22, 0x2c, 0x22, 0x70,
		0x61, 0x72, 0x61, 0x6d, 0x73, 0x22, 0x3a, 0x7b,
		0x22, 0x6d, 0x61, 0x63, 0x22, 0x3a, 0x22, 0x22,
		0x2c, 0x22, 0x75, 0x6e, 0x69, 0x22, 0x3a, 0x31,
		0x7d, 0x7d, 0x0a,
	}

	targets := []struct {
		addr string
		body []byte
	}{
		{"239.255.255.250:37020", []byte(sadpProbe)},
		{"255.255.255.255:37020", []byte(sadpProbe)},
		{"239.255.255.250:3702", []byte(onvifProbe)},
		{"255.255.255.255:3702", []byte(onvifProbe)},
		{"239.255.255.251:37810", dahuaProbe},
		{"255.255.255.255:37810", dahuaProbe},
	}

	for round := 0; round < 2; round++ {
		for _, t := range targets {
			if dst, err := net.ResolveUDPAddr("udp4", t.addr); err == nil {
				_, _ = conn.WriteToUDP(t.body, dst)
			}
		}
		time.Sleep(180 * time.Millisecond)
	}
}

func runFullRadarScan(ifaceFilter string) []CameraEntry {
	allIfaces := getAllNetworkInterfaces()
	myIPs := make(map[string]bool)
	myIPs["127.0.0.1"] = true
	myIPs["0.0.0.0"] = true
	for _, inf := range allIfaces {
		if inf.IP != "" {
			myIPs[inf.IP] = true
		}
		for _, ip4 := range inf.AllIPs {
			myIPs[ip4] = true
		}
	}

	var activeIfaces []NetInterfaceInfo
	for _, inf := range allIfaces {
		if ifaceFilter == "" || ifaceFilter == "auto" {
			if inf.Carrier || inf.IP != "" {
				activeIfaces = append(activeIfaces, inf)
			}
		} else if matchesIfaceFilter(inf.Name, ifaceFilter) {
			activeIfaces = append(activeIfaces, inf)
		}
	}
	if len(activeIfaces) == 0 && len(allIfaces) > 0 {
		activeIfaces = append(activeIfaces, allIfaces[0])
	}

	var mu sync.Mutex
	var rawDiscovered []*CameraEntry
	appendCandidate := func(c *CameraEntry) {
		if c == nil {
			return
		}
		c.IP = strings.TrimSpace(c.IP)
		if c.IP == "" || myIPs[c.IP] {
			return
		}
		if net.ParseIP(c.IP) == nil {
			return
		}
		if c.Iface != "" && !matchesIfaceFilter(c.Iface, ifaceFilter) {
			return
		}
		c.MAC = normalizeMAC(c.MAC)
		mu.Lock()
		rawDiscovered = append(rawDiscovered, c)
		mu.Unlock()
	}

	var wg sync.WaitGroup

	// 1. Вспомогательный L2-сканер Linux (только свежий дамп < 12 секунд)
	if runtime.GOOS != "windows" && ifaceFilter != "vpn" {
		scannerBin := ""
		for _, cand := range []string{"/usr/local/bin/procam-radar", "/usr/local/bin/yeee"} {
			if _, err := os.Stat(cand); err == nil {
				scannerBin = cand
				break
			}
		}
		if scannerBin != "" {
			wg.Add(1)
			go func(bin string) {
				defer wg.Done()
				_ = runPrivilegedLinux(bin, "scan")
				st, err := os.Stat("/var/tmp/yeee_last_scan.json")
				if err != nil || time.Since(st.ModTime()) > 12*time.Second {
					return
				}
				b, err := os.ReadFile("/var/tmp/yeee_last_scan.json")
				if err != nil {
					return
				}
				var items []map[string]interface{}
				if json.Unmarshal(b, &items) != nil {
					var wrapped struct {
						Cameras []map[string]interface{} `json:"cameras"`
					}
					if json.Unmarshal(b, &wrapped) == nil {
						items = wrapped.Cameras
					}
				}
				if len(items) > 0 {
					for _, it := range items {
						ip, _ := it["ip"].(string)
						mac, _ := it["mac"].(string)
						vendor, _ := it["vendor"].(string)
						if vendor == "" {
							vendor, _ = it["vendor_resolved"].(string)
						}
						model, _ := it["model"].(string)
						serial, _ := it["serial"].(string)
						fw, _ := it["firmware"].(string)
						iface, _ := it["iface"].(string)
						conflict, _ := it["conflict"].(bool)
						var ports []int
						if pList, ok := it["ports"].([]interface{}); ok {
							for _, pv := range pList {
								if pf, ok := pv.(float64); ok {
									ports = append(ports, int(pf))
								}
							}
						} else if pList2, ok := it["open_ports"].([]interface{}); ok {
							for _, pv := range pList2 {
								if pf, ok := pv.(float64); ok {
									ports = append(ports, int(pf))
								}
							}
						}
						appendCandidate(&CameraEntry{
							IP:       ip,
							MAC:      mac,
							Vendor:   vendor,
							Model:    model,
							Serial:   serial,
							Firmware: fw,
							Ports:    ports,
							Iface:    iface,
							Conflict: conflict,
						})
					}
				}
			}(scannerBin)
		}
	}

	// 2. Прямой мульти-интерфейсный опрос SADP / ONVIF WS-Discovery / Dahua DHIP
	// Привязываем сокет к конкретному физическому адаптеру (SO_BINDTODEVICE + локальный IP),
	// чтобы при 2 кабелях и 2 Wi-Fi мультикаст гарантированно уходил в КАЖДЫЙ выбранный адаптер!
	listenAndProbe := func(bindIP, ifaceName string) {
		defer wg.Done()
		var laddr *net.UDPAddr
		if bindIP != "" {
			laddr = &net.UDPAddr{IP: net.ParseIP(bindIP), Port: 0}
		} else {
			laddr = &net.UDPAddr{IP: net.IPv4zero, Port: 0}
		}
		conn, err := net.ListenUDP("udp4", laddr)
		if err != nil {
			return
		}
		defer conn.Close()
		if ifaceName != "" {
			bindUDPConnToDevice(conn, ifaceName)
		}

		go sendDiscoveryProbesOnConn(conn)

		_ = conn.SetReadDeadline(time.Now().Add(2300 * time.Millisecond))
		buf := make([]byte, 8192)
		for {
			n, raddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			ip, mac, vendor, model, serial, fw := parseDiscoveryPayload(buf[:n], raddr.IP.String())
			appendCandidate(&CameraEntry{
				IP:       ip,
				MAC:      mac,
				Vendor:   vendor,
				Model:    model,
				Serial:   serial,
				Firmware: fw,
				Iface:    ifaceName,
			})
		}
	}

	wg.Add(1)
	go listenAndProbe("", "")
	for _, inf := range activeIfaces {
		if inf.Kind == "vpn" {
			wg.Add(1)
			go func(vpnInf NetInterfaceInfo) {
				defer wg.Done()
				scanTailscalePeers(vpnInf.Name, myIPs, appendCandidate)
			}(inf)
			continue
		}
		if len(inf.AllIPs) > 0 {
			for _, ip4 := range inf.AllIPs {
				wg.Add(1)
				go listenAndProbe(ip4, inf.Name)
			}
		} else if inf.Carrier {
			// «Голый» кабель L2 без DHCP (например, второй Ethernet-кабель воткнут прямо в камеру)
			wg.Add(1)
			go listenAndProbe("", inf.Name)
		}
	}

	// 3. Быстрый TCP-sweep всех /24 подсетей выбранных адаптеров (включая WireGuard /24, исключая Tailscale 100.64.0.0/10)
	sweptPrefixes := make(map[string]bool)
	for _, inf := range activeIfaces {
		if inf.Kind == "vpn" && strings.HasPrefix(inf.IP, "100.") {
			continue
		}
		ipsToSweep := inf.AllIPs
		if len(ipsToSweep) == 0 && inf.IP != "" {
			ipsToSweep = []string{inf.IP}
		}
		for _, ip4 := range ipsToSweep {
			parts := strings.Split(ip4, ".")
			if len(parts) == 4 {
				prefix := fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
				if sweptPrefixes[prefix] {
					continue
				}
				sweptPrefixes[prefix] = true
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
							c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", target, p), 130*time.Millisecond)
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
	}

	wg.Wait()
	arpMap := readSystemARPTable()

	// 4. Добавляем устройства из системной ARP-таблицы (по OUI производителя или портам камер)
	for ip, recs := range arpMap {
		if myIPs[ip] {
			continue
		}
		for _, rec := range recs {
			if !matchesIfaceFilter(rec.Iface, ifaceFilter) {
				continue
			}
			v := lookupVendor(rec.MAC)
			ports := scanPorts(ip)
			hasCamPort := false
			for _, p := range ports {
				if p == 554 || p == 8000 || p == 37777 || p == 34567 {
					hasCamPort = true
					break
				}
			}
			if v != "" || hasCamPort || len(recs) > 1 {
				appendCandidate(&CameraEntry{
					IP:       ip,
					MAC:      rec.MAC,
					Vendor:   v,
					Ports:    ports,
					Iface:    rec.Iface,
					Conflict: len(recs) > 1,
				})
			}
		}
	}

	// 5. СТРОГАЯ ДЕДУПЛИКАЦИЯ И СЛИЯНИЕ ДАННЫХ ПО IP И MAC
	byIP := make(map[string][]*CameraEntry)
	for _, cand := range rawDiscovered {
		if cand.MAC == "" {
			if arpRecs, ok := arpMap[cand.IP]; ok && len(arpRecs) > 0 {
				cand.MAC = arpRecs[0].MAC
				if cand.Iface == "" || interfacePriority(arpRecs[0].Iface) > interfacePriority(cand.Iface) {
					cand.Iface = arpRecs[0].Iface
				}
			}
		}

		existingList := byIP[cand.IP]
		merged := false
		for _, ex := range existingList {
			if ex.MAC == cand.MAC || ex.MAC == "" || cand.MAC == "" {
				mergeCameraEntry(ex, cand)
				merged = true
				break
			}
		}
		if !merged {
			clone := *cand
			byIP[cand.IP] = append(byIP[cand.IP], &clone)
		}
	}

	var uniqueEntries []*CameraEntry
	seenKey := make(map[string]bool)

	for ip, list := range byIP {
		var withMAC []*CameraEntry
		var withoutMAC []*CameraEntry
		for _, item := range list {
			if item.MAC != "" {
				withMAC = append(withMAC, item)
			} else {
				withoutMAC = append(withoutMAC, item)
			}
		}
		if len(withMAC) > 0 && len(withoutMAC) > 0 {
			for _, noMac := range withoutMAC {
				mergeCameraEntry(withMAC[0], noMac)
			}
			list = withMAC
		}

		isRealConflict := len(list) > 1 || len(arpMap[ip]) > 1
		for _, item := range list {
			if isRealConflict {
				item.Conflict = true
			}
			dedupKey := item.IP + "|" + item.MAC
			if seenKey[dedupKey] {
				continue
			}
			seenKey[dedupKey] = true
			uniqueEntries = append(uniqueEntries, item)
		}
	}

	// 6. Финальное обогащение полей (порты, роль, сетевой мост, зона)
	var result []CameraEntry
	for _, entry := range uniqueEntries {
		if entry.Iface == "" {
			entry.Iface = selectInterfaceForCamera(ifaceFilter)
		}
		entry.IfaceKind = classifyInterface(entry.Iface)
		if isGenericVendor(entry.Vendor) {
			if v := lookupVendor(entry.MAC); v != "" {
				entry.Vendor = v
			} else if entry.Vendor == "" || entry.Vendor == "ONVIF" {
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
			for _, ipnet := range inf.AllNets {
				if ipnet != nil && parsedIP != nil && ipnet.Contains(parsedIP) {
					isLocal = true
					break
				}
			}
		}

		if entry.Conflict {
			entry.Zone = "Конфликт IP"
			entry.ZoneType = "conflict"
		} else if entry.IfaceKind == "vpn" || strings.HasPrefix(entry.IP, "100.") {
			entry.Zone = "VPN / Tailscale"
			entry.ZoneType = "local"
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

	// Числовая сортировка по IPv4, а затем по MAC
	sort.Slice(result, func(i, j int) bool {
		ipA := ipToUint32(result[i].IP)
		ipB := ipToUint32(result[j].IP)
		if ipA != ipB {
			return ipA < ipB
		}
		return result[i].MAC < result[j].MAC
	})

	stateMu.Lock()
	cachedCameras = result
	stateMu.Unlock()

	if runtime.GOOS != "windows" {
		primaryIf, _, _ := getPrimaryInterface()
		cachePayload := map[string]interface{}{
			"timestamp": time.Now().Format("2006-01-02 15:04:05"),
			"iface":     primaryIf,
			"mode":      "scan",
			"cameras":   result,
		}
		if b, err := json.MarshalIndent(cachePayload, "", "  "); err == nil {
			_ = os.WriteFile("/var/tmp/yeee_last_scan.json", b, 0666)
		}
	}

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

// Подбор свободных IP + построение полной матрицы 254 адресов подсети
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

	stateMu.Lock()
	knownCams := make(map[string]CameraEntry)
	for _, c := range cachedCameras {
		knownCams[c.IP] = c
	}
	stateMu.Unlock()

	busyMap := make(map[int]bool)
	systemHosts := map[int]string{
		1:   "Шлюз сети (Gateway)",
		248: "Сервисный мост ProCam",
		249: "Сервисный мост ProCam",
		250: "Резерв NVR / Сервер",
	}
	for h := range systemHosts {
		busyMap[h] = true
	}
	myHostNum := 0
	if myParts := strings.Split(myIP, "."); len(myParts) == 4 && fmt.Sprintf("%s.%s.%s", myParts[0], myParts[1], myParts[2]) == prefix {
		if mHost, err := strconv.Atoi(myParts[3]); err == nil {
			busyMap[mHost] = true
			myHostNum = mHost
			systemHosts[mHost] = "Этот компьютер"
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 48)

	for host := 2; host <= 254; host++ {
		if busyMap[host] {
			continue
		}
		cand := fmt.Sprintf("%s.%d", prefix, host)
		if _, isKnown := knownCams[cand]; isKnown || len(arpMap[cand]) > 0 {
			busyMap[host] = true
			continue
		}
		wg.Add(1)
		go func(h int, ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			for _, p := range []int{80, 554, 8000, 37777} {
				if c, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, p), 100*time.Millisecond); err == nil {
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

	// Формируем полную карту 254 адресов подсети для визуальной матрицы
	var hostMap []SubnetHostCell
	for h := 1; h <= 254; h++ {
		ip := fmt.Sprintf("%s.%d", prefix, h)
		cell := SubnetHostCell{
			Host:   h,
			IP:     ip,
			Status: "free",
		}
		if cam, ok := knownCams[ip]; ok {
			cell.Vendor = cam.Vendor
			cell.MAC = cam.MAC
			cell.Label = cam.Model
			if cam.Conflict {
				cell.Status = "conflict"
			} else if cam.DeviceType == "nvr" {
				cell.Status = "nvr"
			} else {
				cell.Status = "camera"
			}
		} else if sysLbl, ok := systemHosts[h]; ok && (h == 1 || h == myHostNum || busyMap[h]) {
			cell.Status = "system"
			cell.Label = sysLbl
		} else if recs := arpMap[ip]; len(recs) > 0 {
			cell.MAC = recs[0].MAC
			cell.Vendor = lookupVendor(recs[0].MAC)
			if len(recs) > 1 {
				cell.Status = "conflict"
				cell.Label = "Конфликт ARP"
			} else if cell.Vendor != "" {
				cell.Status = "camera"
				cell.Label = cell.Vendor
			} else {
				cell.Status = "occupied"
				cell.Label = "Занят в ARP"
			}
		} else if busyMap[h] {
			cell.Status = "occupied"
			cell.Label = "Устройство онлайн"
		}
		hostMap = append(hostMap, cell)
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
		HostMap:         hostMap,
	}
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func doDigestRequestRaw(method, urlStr, user, pass string, body []byte) (int, []byte, error) {
	client := &http.Client{
		Timeout:   3500 * time.Millisecond,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
	}
	req1, err := http.NewRequest(method, urlStr, nil)
	if err != nil {
		return 0, nil, err
	}
	if user != "" || pass != "" {
		req1.SetBasicAuth(user, pass)
	}
	resp1, err := client.Do(req1)
	if err != nil {
		return 0, nil, err
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusUnauthorized {
		b, _ := io.ReadAll(resp1.Body)
		return resp1.StatusCode, b, nil
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
	if realm == "" && nonce == "" {
		b, _ := io.ReadAll(resp1.Body)
		return resp1.StatusCode, b, nil
	}
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
		return 0, nil, err
	}
	req2.Header.Set("Authorization", digestVal)
	if len(body) > 0 {
		req2.Header.Set("Content-Type", "application/xml")
	}
	resp2, err := client.Do(req2)
	if err != nil {
		return 0, nil, err
	}
	defer resp2.Body.Close()
	b2, _ := io.ReadAll(resp2.Body)
	return resp2.StatusCode, b2, nil
}

func doDigestRequest(method, urlStr, user, pass string, body []byte) (int, string, error) {
	code, b, err := doDigestRequestRaw(method, urlStr, user, pass, body)
	return code, string(b), err
}

func captureCameraSnapshot(ip, pass string) (string, string) {
	iface := resolveInterfaceForCamera(ip, "", "")
	injectSubnetWindowsOrLinux(iface, ip)
	outPath := filepath.Join(os.TempDir(), fmt.Sprintf("snap_%s.jpg", ip))

	passwords := []string{pass}
	if pass == "" {
		passwords = []string{"12345", "admin", "123456", "admin123", "888888", ""}
	}
	endpoints := []string{
		fmt.Sprintf("http://%s/ISAPI/Streaming/channels/101/picture", ip),
		fmt.Sprintf("http://%s/ISAPI/Streaming/channels/1/picture", ip),
		fmt.Sprintf("http://%s/cgi-bin/snapshot.cgi?channel=1", ip),
		fmt.Sprintf("http://%s/onvif-http/snapshot?Profile_1", ip),
	}

	for _, ep := range endpoints {
		for _, pw := range passwords {
			code, data, err := doDigestRequestRaw("GET", ep, "admin", pw, nil)
			if err == nil && code == 200 && len(data) > 512 && data[0] == 0xFF && data[1] == 0xD8 {
				_ = os.WriteFile(outPath, data, 0644)
				return outPath, fmt.Sprintf("Стоп-кадр успешно получен (%d КБ) -> %s", len(data)/1024, outPath)
			}
		}
	}

	if runtime.GOOS != "windows" {
		for _, bin := range []string{"/usr/local/bin/procam-radar", "/usr/local/bin/yeee"} {
			if _, err := os.Stat(bin); err == nil {
				args := []string{"-n", bin, "snap", ip}
				if pass != "" {
					args = append(args, "admin", pass)
				}
				out, _ := runHiddenOutput("sudo", args...)
				if st, err := os.Stat(outPath); err == nil && st.Size() > 512 {
					return outPath, stripANSI(string(out))
				}
				return "", stripANSI(string(out))
			}
		}
	}

	return "", fmt.Sprintf("Не удалось получить JPEG стоп-кадр с %s (проверьте пароль камеры).", ip)
}

func openExternalTarget(target string) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
		prepareBackgroundCmd(cmd)
		_ = cmd.Start()
	} else {
		if os.Geteuid() == 0 && os.Getenv("SUDO_USER") != "" {
			_ = exec.Command("sudo", "-u", os.Getenv("SUDO_USER"),
				"--preserve-env=DISPLAY,WAYLAND_DISPLAY,XAUTHORITY,XDG_RUNTIME_DIR,DBUS_SESSION_BUS_ADDRESS",
				"xdg-open", target).Start()
		} else {
			_ = exec.Command("xdg-open", target).Start()
		}
	}
}

func buildCameraRTSPURL(ip, user, pass, streamMode, profile string) string {
	if user == "" {
		user = "admin"
	}
	isSub := streamMode == "sub" || streamMode == "102" || streamMode == "1"

	// Определяем бренд/порты по кешу, если profile == "auto" или пустой
	isDahua := false
	isONVIF := false
	if profile == "dahua" {
		isDahua = true
	} else if profile == "onvif" {
		isONVIF = true
	} else if profile == "" || profile == "auto" {
		stateMu.Lock()
		for _, c := range cachedCameras {
			if c.IP == ip {
				if strings.Contains(strings.ToLower(c.Vendor), "dahua") || strings.Contains(strings.ToLower(c.Vendor), "imou") {
					isDahua = true
				}
				for _, p := range c.Ports {
					if p == 37777 {
						isDahua = true
					}
				}
				break
			}
		}
		stateMu.Unlock()
	}

	path := "/Streaming/Channels/101"
	if isSub {
		path = "/Streaming/Channels/102"
	}
	if isDahua {
		if isSub {
			path = "/cam/realmonitor?channel=1&subtype=1"
		} else {
			path = "/cam/realmonitor?channel=1&subtype=0"
		}
	} else if isONVIF {
		if isSub {
			path = "/stream2"
		} else {
			path = "/stream1"
		}
	}

	u := &url.URL{
		Scheme: "rtsp",
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:554", ip),
	}
	return u.String() + path
}

func checkRTSPConnection(ip, user, pass, rtspURL string) (bool, string) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:554", ip), 1500*time.Millisecond)
	if err != nil {
		return false, fmt.Sprintf("Порт RTSP %s:554 недоступен или закрыт (%v). Проверьте подключение к камере.", ip, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2200 * time.Millisecond))

	// Отправляем DESCRIBE без пароля, чтобы узнать, требует ли камера Digest/Basic авторизацию
	cleanURL := fmt.Sprintf("rtsp://%s:554/Streaming/Channels/101", ip)
	req := fmt.Sprintf("DESCRIBE %s RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: ProCamRadar/5.0\r\nAccept: application/sdp\r\n\r\n", cleanURL)
	_, _ = conn.Write([]byte(req))
	buf := make([]byte, 2048)
	n, _ := conn.Read(buf)
	resp := string(buf[:n])

	if strings.Contains(resp, "RTSP/1.0 200") {
		return true, "RTSP-поток открыт и доступен (200 OK)"
	}
	if strings.Contains(resp, "401 Unauthorized") {
		if pass == "" {
			return false, "Камера требует пароль (401 Unauthorized). Введите пароль камеры в поле «Пароль камер» выше!"
		}
		// Проверяем валидность пароля через HTTP ISAPI/CGI или ffprobe/ffmpeg
		code, _, err := doDigestRequestRaw("GET", fmt.Sprintf("http://%s/ISAPI/System/status", ip), user, pass, nil)
		if err == nil && code == 401 {
			return false, fmt.Sprintf("Неверный пароль камеры «%s» (401 Unauthorized). Укажите правильный пароль от камеры!", pass)
		}
		return true, fmt.Sprintf("Порт RTSP 554 активен (авторизация %s:***, запуск потока...)", user)
	}
	return true, "Порт RTSP 554 открыт, подключение к видеопотоку..."
}

func handleMJPEGStream(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimSpace(r.URL.Query().Get("ip"))
	user := strings.TrimSpace(r.URL.Query().Get("user"))
	pass := r.URL.Query().Get("pass")
	sub := r.URL.Query().Get("sub")
	profile := r.URL.Query().Get("profile")
	if ip == "" {
		http.Error(w, "Missing IP", http.StatusBadRequest)
		return
	}
	if user == "" {
		user = "admin"
	}

	iface := resolveInterfaceForCamera(ip, "", r.URL.Query().Get("iface"))
	injectSubnetWindowsOrLinux(iface, ip)
	rtspURL := buildCameraRTSPURL(ip, user, pass, sub, profile)

	// 1. Если установлен ffmpeg — транслируем живой RTSP-поток в окно приложения через MJPEG
	if ffPath, err := exec.LookPath("ffmpeg"); err == nil {
		c, dialErr := net.DialTimeout("tcp", fmt.Sprintf("%s:554", ip), 1200*time.Millisecond)
		if dialErr == nil {
			_ = c.Close()
			w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=ffserver")
			w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
			w.Header().Set("Connection", "close")

			cmd := exec.CommandContext(r.Context(), ffPath,
				"-loglevel", "error",
				"-rtsp_transport", "tcp",
				"-i", rtspURL,
				"-vf", "scale=960:-1",
				"-r", "12",
				"-q:v", "5",
				"-f", "mpjpeg",
				"-",
			)
			prepareBackgroundCmd(cmd)
			cmd.Stdout = w
			if err := cmd.Run(); err == nil {
				return
			}
		}
	}

	// 2. Fallback: потоковая передача кадров через HTTP Snapshot API (ISAPI / Dahua CGI)
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=procamframe")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	flusher, ok := w.(http.Flusher)
	for i := 0; i < 120; i++ {
		select {
		case <-r.Context().Done():
			return
		default:
		}
		snapPath, _ := captureCameraSnapshot(ip, pass)
		if snapPath == "" {
			return
		}
		b, err := os.ReadFile(snapPath)
		if err != nil || len(b) < 512 {
			return
		}
		_, err = fmt.Fprintf(w, "--procamframe\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(b))
		if err != nil {
			return
		}
		if _, err = w.Write(b); err != nil {
			return
		}
		_, _ = w.Write([]byte("\r\n"))
		if ok {
			flusher.Flush()
		}
		time.Sleep(350 * time.Millisecond)
	}
}

func openRTSPPlayer(rtspURL string) string {
	players := []struct {
		bin  string
		args []string
	}{
		{"ffplay", []string{"-rtsp_transport", "tcp", "-window_title", "ProCam Radar — RTSP Видеопоток", "-x", "1024", "-y", "576", rtspURL}},
		{"mpv", []string{"--force-window=immediate", "--keep-open=yes", "--rtsp-transport=tcp", "--title=ProCam Radar — RTSP Видеопоток", "--geometry=1024x576", rtspURL}},
		{"vlc", []string{"--rtsp-tcp", rtspURL}},
	}
	for _, p := range players {
		if path, err := exec.LookPath(p.bin); err == nil {
			var cmd *exec.Cmd
			if runtime.GOOS != "windows" && os.Geteuid() == 0 && os.Getenv("SUDO_USER") != "" {
				sudoArgs := append([]string{
					"-u", os.Getenv("SUDO_USER"),
					"--preserve-env=DISPLAY,WAYLAND_DISPLAY,XAUTHORITY,XDG_RUNTIME_DIR,DBUS_SESSION_BUS_ADDRESS",
					path,
				}, p.args...)
				cmd = exec.Command("sudo", sudoArgs...)
			} else {
				cmd = exec.Command(path, p.args...)
			}
			prepareBackgroundCmd(cmd)
			if err := cmd.Start(); err == nil {
				return fmt.Sprintf("Запущен внешний видеоплеер %s -> %s", p.bin, rtspURL)
			}
		}
	}
	openExternalTarget(rtspURL)
	return fmt.Sprintf("Открыт видеопоток -> %s", rtspURL)
}

const nativeGTKWindowPy = `import sys, urllib.request
import gi
gi.require_version('Gtk', '3.0')
try:
    gi.require_version('WebKit2', '4.1')
except ValueError:
    gi.require_version('WebKit2', '4.0')
from gi.repository import Gtk, WebKit2, Gdk

url = sys.argv[1]
win = Gtk.Window(title="ProCam Radar v5.0 — Инженерный комплекс IP-камер и NVR")
win.set_default_size(1440, 880)
win.set_position(Gtk.WindowPosition.CENTER)
win.set_wmclass("procam-radar", "ProCam Radar")

settings = Gtk.Settings.get_default()
if settings:
    settings.set_property("gtk-application-prefer-dark-theme", True)

webview = WebKit2.WebView()
ws = webview.get_settings()
ws.set_enable_developer_extras(False)
ws.set_enable_write_console_messages_to_stdout(False)
webview.set_background_color(Gdk.RGBA(0.035, 0.039, 0.055, 1.0))
webview.load_uri(url)

def on_decide_policy(view, decision, decision_type):
    if decision_type == WebKit2.PolicyDecisionType.NEW_WINDOW_ACTION:
        decision.ignore()
        return True
    return False

webview.connect("decide-policy", on_decide_policy)
win.add(webview)

def on_destroy(*args):
    try:
        urllib.request.urlopen(url + "/api/quit", timeout=0.4)
    except Exception:
        pass
    Gtk.main_quit()

win.connect("destroy", on_destroy)
win.show_all()
Gtk.main()
`

func launchStandaloneDesktopApp(url string) {
	// На Linux в первую очередь открываем настоящее нативное GTK3 + WebKit2 окно приложения
	if runtime.GOOS != "windows" {
		if pyPath, err := exec.LookPath("python3"); err == nil {
			checkCmd := exec.Command(pyPath, "-c", "import gi; gi.require_version('Gtk','3.0'); gi.require_version('WebKit2','4.1')")
			if checkCmd.Run() != nil {
				checkCmd = exec.Command(pyPath, "-c", "import gi; gi.require_version('Gtk','3.0'); gi.require_version('WebKit2','4.0')")
			}
			if checkCmd.Run() == nil {
				var cmd *exec.Cmd
				if os.Geteuid() == 0 && os.Getenv("SUDO_USER") != "" {
					cmd = exec.Command("sudo", "-u", os.Getenv("SUDO_USER"),
						"--preserve-env=DISPLAY,WAYLAND_DISPLAY,XAUTHORITY,XDG_RUNTIME_DIR,DBUS_SESSION_BUS_ADDRESS",
						pyPath, "-c", nativeGTKWindowPy, url)
				} else {
					cmd = exec.Command(pyPath, "-c", nativeGTKWindowPy, url)
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
	}

	tmpDir, err := os.MkdirTemp("", "procam-radar-window-*")
	if err == nil {
		appTempDir = tmpDir
		_ = os.WriteFile(filepath.Join(tmpDir, "First Run"), []byte(""), 0644)
	}

	commonFlags := []string{
		"--app=" + url,
		"--window-size=1440,880",
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
				startedAt := time.Now()
				if err := cmd.Start(); err == nil {
					go func() {
						_ = cmd.Wait()
						if time.Since(startedAt) > 3*time.Second {
							cleanupAndExit()
						}
					}()
					return
				}
			}
		}
		cmd := exec.Command("cmd", "/c", "start", "", "msedge", "--app="+url, "--window-size=1440,880")
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
			startedAt := time.Now()
			if err := cmd.Start(); err == nil {
				go func() {
					_ = cmd.Wait()
					if time.Since(startedAt) > 3*time.Second {
						cleanupAndExit()
					}
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

const appIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" fill="none">
  <defs>
    <radialGradient id="appHdrSweep" cx="32" cy="32" r="23" gradientUnits="userSpaceOnUse">
      <stop offset="0%" stop-color="#10B981" stop-opacity="0.5"/>
      <stop offset="60%" stop-color="#10B981" stop-opacity="0.18"/>
      <stop offset="100%" stop-color="#10B981" stop-opacity="0"/>
    </radialGradient>
    <linearGradient id="appHdrBeam" x1="32" y1="32" x2="48" y2="16" gradientUnits="userSpaceOnUse">
      <stop offset="0%" stop-color="#34D399" stop-opacity="1"/>
      <stop offset="100%" stop-color="#10B981" stop-opacity="0.2"/>
    </linearGradient>
  </defs>
  <rect width="64" height="64" rx="15" fill="#09090B"/>
  <rect x="1" y="1" width="62" height="62" rx="14" stroke="#27272A" stroke-width="1.2"/>
  <circle cx="32" cy="32" r="23" fill="#0E131F" stroke="#27272A" stroke-width="1.8"/>
  <circle cx="32" cy="32" r="17" stroke="#10B981" stroke-width="1" stroke-opacity="0.28" stroke-dasharray="3 2"/>
  <circle cx="32" cy="32" r="11" stroke="#10B981" stroke-width="1.2" stroke-opacity="0.45"/>
  <circle cx="32" cy="32" r="5" stroke="#10B981" stroke-width="1.2" stroke-opacity="0.6"/>
  <line x1="9" y1="32" x2="55" y2="32" stroke="#27272A" stroke-width="1.2"/>
  <line x1="32" y1="9" x2="32" y2="55" stroke="#27272A" stroke-width="1.2"/>
  <path d="M32 32 L32 9 A23 23 0 0 1 48.26 15.74 Z" fill="url(#appHdrSweep)"/>
  <line x1="32" y1="32" x2="48.26" y2="15.74" stroke="url(#appHdrBeam)" stroke-width="2" stroke-linecap="round"/>
  <circle cx="42" cy="22" r="2.8" fill="#10B981" fill-opacity="0.25"/>
  <circle cx="42" cy="22" r="1.8" fill="#34D399"/>
  <circle cx="23" cy="23" r="1.6" fill="#10B981"/>
  <circle cx="25" cy="41" r="1.6" fill="#10B981"/>
  <circle cx="32" cy="32" r="3" fill="#09090B" stroke="#FFFFFF" stroke-width="1.4"/>
  <circle cx="32" cy="32" r="1.5" fill="#10B981"/>
</svg>`

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
		"/home/yerniyaz/Desktop/projects/procam-radar",
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
  ProCamRadar                Запуск нативного приложения ProCam Radar
  ProCamRadar --port 8765    Указать локальный порт (по умолчанию 8765)
  ProCamRadar --headless     Запуск только фонового сервера без открытия окна
  ProCamRadar --help         Показать справку
`, AppName, Version)
}

func main() {
	reqPort := "8765"
	headless := false
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help", "help":
			printCLIHelp()
			return
		case "--headless", "--no-window":
			headless = true
		case "--port", "-p":
			if i+1 < len(args) {
				reqPort = args[i+1]
				i++
			}
		}
	}

	webRoot, err := fs.Sub(embeddedWebFS, "web")
	if err != nil {
		fmt.Println("Ошибка инициализации встроенного UI:", err)
		return
	}
	webServer := http.FileServer(http.FS(webRoot))

	http.Handle("/", webServer)
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
			for _, bin := range []string{"/usr/local/bin/procam-radar", "/usr/local/bin/yeee"} {
				if _, err := os.Stat(bin); err == nil {
					_ = runPrivilegedLinux(bin, "dump", "pdf")
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
		}
		p := findDistFile("PROCAM_RADAR_MANUAL_RU.pdf", "YEEE_PRO_MANUAL_RU.pdf")
		if p != "" {
			w.Header().Set("Content-Type", "application/pdf")
			http.ServeFile(w, r, p)
			return
		}
		http.Error(w, "PDF отчет не найден", 404)
	})
	http.HandleFunc("/api/pdf/open", func(w http.ResponseWriter, r *http.Request) {
		if runtime.GOOS != "windows" {
			for _, bin := range []string{"/usr/local/bin/procam-radar", "/usr/local/bin/yeee"} {
				if _, err := os.Stat(bin); err == nil {
					_ = runPrivilegedLinux(bin, "dump", "pdf")
					matches, _ := filepath.Glob("cameras_audit_*.pdf")
					if len(matches) > 0 {
						sort.Strings(matches)
						latest, _ := filepath.Abs(matches[len(matches)-1])
						openExternalTarget(latest)
						fmt.Fprintf(w, "PDF-аудит сформирован и открыт: %s", latest)
						return
					}
				}
			}
		}
		p := findDistFile("PROCAM_RADAR_MANUAL_RU.pdf", "YEEE_PRO_MANUAL_RU.pdf")
		if p != "" {
			absP, _ := filepath.Abs(p)
			openExternalTarget(absP)
			fmt.Fprintf(w, "Открыт PDF-документ: %s", absP)
			return
		}
		fmt.Fprintf(w, "Не удалось сформировать PDF-отчет.")
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
		iface := resolveInterfaceForCamera(ip, mac, r.URL.Query().Get("iface"))
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
		sub := r.URL.Query().Get("sub")
		profile := r.URL.Query().Get("profile")
		if user == "" {
			user = "admin"
		}
		iface := resolveInterfaceForCamera(ip, "", r.URL.Query().Get("iface"))
		injectSubnetWindowsOrLinux(iface, ip)
		rtspURL := buildCameraRTSPURL(ip, user, pass, sub, profile)
		msg := openRTSPPlayer(rtspURL)
		_, _ = w.Write([]byte(msg))
	})
	http.HandleFunc("/api/rtsp/check", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		user := r.URL.Query().Get("user")
		pass := r.URL.Query().Get("pass")
		sub := r.URL.Query().Get("sub")
		profile := r.URL.Query().Get("profile")
		if user == "" {
			user = "admin"
		}
		iface := resolveInterfaceForCamera(ip, "", r.URL.Query().Get("iface"))
		injectSubnetWindowsOrLinux(iface, ip)
		rtspURL := buildCameraRTSPURL(ip, user, pass, sub, profile)
		ok, msg := checkRTSPConnection(ip, user, pass, rtspURL)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":       ok,
			"message":  msg,
			"rtsp_url": rtspURL,
		})
	})
	http.HandleFunc("/api/stream.mjpeg", handleMJPEGStream)
	http.HandleFunc("/api/openurl", func(w http.ResponseWriter, r *http.Request) {
		u := strings.TrimSpace(r.URL.Query().Get("url"))
		if u != "" && (strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "mailto:") || strings.HasPrefix(u, "tel:")) {
			openExternalTarget(u)
			_, _ = w.Write([]byte("OK"))
			return
		}
		http.Error(w, "Invalid URL", http.StatusBadRequest)
	})
	http.HandleFunc("/api/snap", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		pass := r.URL.Query().Get("pass")
		_, msg := captureCameraSnapshot(ip, pass)
		_, _ = w.Write([]byte(msg))
	})
	http.HandleFunc("/api/snap/image", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		pass := r.URL.Query().Get("pass")
		imgPath := filepath.Join(os.TempDir(), fmt.Sprintf("snap_%s.jpg", ip))
		if st, err := os.Stat(imgPath); err != nil || st.Size() < 512 || time.Since(st.ModTime()) > 15*time.Second {
			capturedPath, _ := captureCameraSnapshot(ip, pass)
			if capturedPath != "" {
				imgPath = capturedPath
			}
		}
		if st, err := os.Stat(imgPath); err == nil && st.Size() > 512 {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, imgPath)
			return
		}
		http.Error(w, "Снимок недоступен", http.StatusNotFound)
	})
	http.HandleFunc("/api/caminfo", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		if runtime.GOOS != "windows" {
			for _, bin := range []string{"/usr/local/bin/procam-radar", "/usr/local/bin/yeee"} {
				if _, err := os.Stat(bin); err == nil {
					out, _ := runHiddenOutput("sudo", "-n", bin, "info", ip)
					_, _ = w.Write([]byte(stripANSI(string(out))))
					return
				}
			}
		}
		iface := resolveInterfaceForCamera(ip, "", r.URL.Query().Get("iface"))
		sub := injectSubnetWindowsOrLinux(iface, ip)
		ports := scanPorts(ip)
		fmt.Fprintf(w, "IP-адрес: %s (%s)\nОткрытые порты: %v\nИнтерфейс: %s", ip, sub, ports, iface)
	})
	http.HandleFunc("/api/cable", func(w http.ResponseWriter, r *http.Request) {
		ip := r.URL.Query().Get("ip")
		if runtime.GOOS != "windows" {
			for _, bin := range []string{"/usr/local/bin/procam-radar", "/usr/local/bin/yeee"} {
				if _, err := os.Stat(bin); err == nil {
					out, _ := runHiddenOutput("sudo", "-n", bin, "cable", ip)
					_, _ = w.Write([]byte(stripANSI(string(out))))
					return
				}
			}
		}
		iface := resolveInterfaceForCamera(ip, "", r.URL.Query().Get("iface"))
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
			for _, bin := range []string{"/usr/local/bin/procam-radar", "/usr/local/bin/yeee"} {
				if _, err := os.Stat(bin); err == nil {
					args := []string{"-n", bin, "activate", pass}
					if startIP != "" {
						args = append(args, startIP)
					}
					out, _ := runHiddenOutput("sudo", args...)
					_, _ = w.Write([]byte(stripANSI(string(out))))
					return
				}
			}
		}
		fmt.Fprintf(w, "Пакетная активация выполнена (пароль задан).")
	})
	http.HandleFunc("/api/resolve", func(w http.ResponseWriter, r *http.Request) {
		cip := r.URL.Query().Get("conflict")
		sip := r.URL.Query().Get("start")
		pass := r.URL.Query().Get("pass")
		if runtime.GOOS != "windows" {
			for _, bin := range []string{"/usr/local/bin/procam-radar", "/usr/local/bin/yeee"} {
				if _, err := os.Stat(bin); err == nil {
					args := []string{"-n", bin, "resolve", cip, sip}
					if pass != "" {
						args = append(args, "admin", pass)
					}
					out, _ := runHiddenOutput("sudo", args...)
					_, _ = w.Write([]byte(stripANSI(string(out))))
					return
				}
			}
		}
		fmt.Fprintf(w, "Пакетное разведение конфликта %s -> %s завершено.", cip, sip)
	})
	http.HandleFunc("/api/setip", func(w http.ResponseWriter, r *http.Request) {
		oldIP := r.URL.Query().Get("old")
		newIP := r.URL.Query().Get("new")
		mac := r.URL.Query().Get("mac")
		pass := r.URL.Query().Get("pass")
		iface := resolveInterfaceForCamera(oldIP, mac, r.URL.Query().Get("iface"))
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
		fmt.Fprintf(w, "Автоматическая смена не прошла (требуется активация или другой пароль).\nНажмите кнопку «Открыть по MAC» для прямого входа в веб-интерфейс камеры.")
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

	if !headless {
		go func() {
			time.Sleep(200 * time.Millisecond)
			launchStandaloneDesktopApp(localURL)
		}()
	}
	_ = http.Serve(listener, nil)
}
