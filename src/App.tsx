import React, { useEffect, useMemo, useState } from 'react';
import {
  CameraEntry,
  FreePoolResponse,
  HostCell,
  NetInterfaceInfo,
  SysInfoResponse,
} from './types';

function ipToNumber(ip: string): number {
  const parts = (ip || '').trim().split('.');
  if (parts.length !== 4) return 0xffffffff;
  return (
    ((parseInt(parts[0], 10) & 255) << 24) >>> 0 |
    ((parseInt(parts[1], 10) & 255) << 16) |
    ((parseInt(parts[2], 10) & 255) << 8) |
    (parseInt(parts[3], 10) & 255)
  ) >>> 0;
}

function normalizeMac(mac?: string): string {
  if (!mac) return '';
  const m = mac.trim().replace(/-/g, ':').toUpperCase();
  if (m === '00:00:00:00:00:00' || m === 'FF:FF:FF:FF:FF:FF' || m === '-') return '';
  return m;
}

function cleanModelName(model?: string): string {
  if (!model) return '';
  const m = model.trim();
  const low = m.toLowerCase();
  if (
    low === 'dn:networkvideotransmitter' ||
    low === 'tds:device' ||
    low === 'dn:networkvideotransmitter tds:device' ||
    low.startsWith('н/д') ||
    low === 'n/a' ||
    low === '-'
  ) {
    return '';
  }
  return m;
}

function isBetterVendor(candidate: string, current: string): boolean {
  if (!candidate) return false;
  if (!current) return true;
  const generic = ['onvif device', 'onvif ip camera', 'ip-камера (onvif)', 'неизвестный бренд'];
  const candLow = candidate.toLowerCase();
  const curLow = current.toLowerCase();
  const candIsGen = generic.some((g) => candLow.includes(g));
  const curIsGen = generic.some((g) => curLow.includes(g));
  if (curIsGen && !candIsGen) return true;
  return candidate.length > current.length && curIsGen === candIsGen;
}

/**
 * Guarantees 100% deduplication of cameras:
 * - Normalizes MAC addresses.
 * - Merges entries for the same IP when one entry has an empty MAC (e.g. ONVIF UDP response)
 *   and another entry has the hardware MAC (e.g. ARP / SADP / DHIP).
 * - Merges entries with identical (IP, MAC) pairs.
 * - Preserves genuine IP conflicts ONLY when two distinct non-empty MAC addresses share one IP.
 * - Sorts numerically by IPv4 address.
 */
function deduplicateAndSortCameras(rawList: CameraEntry[]): CameraEntry[] {
  const byIP = new Map<string, CameraEntry[]>();

  for (const raw of rawList || []) {
    if (!raw || !raw.ip || raw.ip === '0.0.0.0') continue;
    const ip = raw.ip.trim();
    const mac = normalizeMac(raw.mac);
    const cleanedModel = cleanModelName(raw.model);

    const item: CameraEntry = {
      ...raw,
      ip,
      mac,
      model: cleanedModel || (raw.device_type === 'nvr' ? 'NVR / Регистратор' : 'IP-Камера'),
      ports: Array.isArray(raw.ports) ? Array.from(new Set(raw.ports)).sort((a, b) => a - b) : [],
    };

    const existingList = byIP.get(ip) || [];
    if (existingList.length === 0) {
      byIP.set(ip, [item]);
      continue;
    }

    // Try to find matching entry to merge into:
    // 1. Exact same MAC
    // 2. New item has empty MAC -> merge into first existing entry for this IP
    // 3. Existing entry has empty MAC -> assign new MAC and merge into it
    let merged = false;
    for (const ex of existingList) {
      if (ex.mac === mac || !mac || !ex.mac) {
        if (!ex.mac && mac) ex.mac = mac;
        if (isBetterVendor(item.vendor, ex.vendor)) ex.vendor = item.vendor;
        const exCleanModel = cleanModelName(ex.model);
        if (cleanedModel && (!exCleanModel || exCleanModel === 'IP-Камера' || exCleanModel === 'IPC')) {
          ex.model = cleanedModel;
        }
        if (item.serial && item.serial !== 'Н/Д' && (!ex.serial || ex.serial === 'Н/Д')) {
          ex.serial = item.serial;
        }
        if (item.firmware && item.firmware !== 'Н/Д' && (!ex.firmware || ex.firmware === 'Н/Д')) {
          ex.firmware = item.firmware;
        }
        if (item.device_type === 'nvr') {
          ex.device_type = 'nvr';
          ex.device_label = 'NVR / Регистратор';
        }
        ex.ports = Array.from(new Set([...(ex.ports || []), ...(item.ports || [])])).sort(
          (a, b) => a - b
        );
        if (!ex.iface && item.iface) ex.iface = item.iface;
        if (!ex.subnet_alias && item.subnet_alias) ex.subnet_alias = item.subnet_alias;
        if (item.ping_ms && (!ex.ping_ms || item.ping_ms < ex.ping_ms)) ex.ping_ms = item.ping_ms;
        merged = true;
        break;
      }
    }

    if (!merged) {
      existingList.push(item);
    }
    byIP.set(ip, existingList);
  }

  const finalResult: CameraEntry[] = [];
  const seenKeys = new Set<string>();

  for (const [, entries] of byIP.entries()) {
    const hasRealConflict = entries.length > 1;
    for (const e of entries) {
      const key = `${e.ip}|${e.mac || 'NOMAC'}`;
      if (seenKeys.has(key)) continue;
      seenKeys.add(key);
      if (hasRealConflict) {
        e.conflict = true;
        e.zone_type = 'conflict';
        e.zone = 'Конфликт IP';
      }
      finalResult.push(e);
    }
  }

  finalResult.sort((a, b) => {
    const diff = ipToNumber(a.ip) - ipToNumber(b.ip);
    if (diff !== 0) return diff;
    return (a.mac || '').localeCompare(b.mac || '');
  });

  return finalResult;
}

type NavTab = 'radar' | 'cable' | 'ipmap' | 'batch' | 'calc' | 'help' | 'about';
type InspectorTab = 'control' | 'passport' | 'cable';
type FilterMode = 'all' | 'camera' | 'nvr' | 'local' | 'foreign' | 'conflict';

export default function App() {
  const [theme, setTheme] = useState<'dark' | 'light'>('dark');
  const [navTab, setNavTab] = useState<NavTab>('radar');
  const [inspectorTab, setInspectorTab] = useState<InspectorTab>('control');

  // System & Network State
  const [sysInfo, setSysInfo] = useState<SysInfoResponse | null>(null);
  const [ifaceMode, setIfaceMode] = useState<string>('auto');

  // Cameras State
  const [cameras, setCameras] = useState<CameraEntry[]>([]);
  const [scanning, setScanning] = useState<boolean>(false);
  const [plugScanning, setPlugScanning] = useState<boolean>(false);
  const [plugCountdown, setPlugCountdown] = useState<number>(24);
  const [filterMode, setFilterMode] = useState<FilterMode>('all');
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [sortBy, setSortBy] = useState<'ip' | 'vendor' | 'model' | 'type' | 'zone'>('ip');
  const [sortAsc, setSortAsc] = useState<boolean>(true);

  // Selected Camera & Workbench State
  const [selectedIP, setSelectedIP] = useState<string>('192.168.1.64');
  const [selectedMAC, setSelectedMAC] = useState<string>('');
  const [newIP, setNewIP] = useState<string>('');
  const [camUser, setCamUser] = useState<string>(
    () => localStorage.getItem('procam_cam_user') || 'admin'
  );
  const [camPass, setCamPass] = useState<string>(
    () => localStorage.getItem('procam_cam_pass') || ''
  );
  const [showPass, setShowPass] = useState<boolean>(false);

  // Live RTSP Video Modal State
  const [rtspModalOpen, setRtspModalOpen] = useState<boolean>(false);
  const [rtspStreamSub, setRtspStreamSub] = useState<'main' | 'sub'>('main');
  const [rtspProfile, setRtspProfile] = useState<'auto' | 'hikvision' | 'dahua' | 'onvif'>('auto');
  const [rtspStatus, setRtspStatus] = useState<{
    ok: boolean;
    message: string;
    rtsp_url: string;
  } | null>(null);
  const [rtspChecking, setRtspChecking] = useState<boolean>(false);
  const [mjpegTick, setMjpegTick] = useState<number>(0);
  const [mjpegError, setMjpegError] = useState<boolean>(false);

  useEffect(() => {
    try {
      localStorage.setItem('procam_cam_user', camUser);
      localStorage.setItem('procam_cam_pass', camPass);
    } catch {
      // ignore storage errors
    }
  }, [camUser, camPass]);

  // Inspector Output State
  const [diagLoading, setDiagLoading] = useState<boolean>(false);
  const [passportText, setPassportText] = useState<string>('');
  const [cableText, setCableText] = useState<string>('');
  const [cableStats, setCableStats] = useState<{
    smallLoss: number;
    heavyLoss: number;
    verdict: string;
    status: 'ok' | 'warning' | 'bad';
  } | null>(null);
  const [snapshotUrl, setSnapshotUrl] = useState<string>('');
  const [statusMessage, setStatusMessage] = useState<string>('Готов к работе');

  // Free IP Pool & 254 Matrix State
  const [freeCount, setFreeCount] = useState<number>(8);
  const [freeSubnet, setFreeSubnet] = useState<string>('');
  const [freeLoading, setFreeLoading] = useState<boolean>(false);
  const [freePool, setFreePool] = useState<FreePoolResponse | null>(null);

  // Batch Activation & Conflict Resolution State
  const [batchPass, setBatchPass] = useState<string>('');
  const [batchStartIP, setBatchStartIP] = useState<string>('192.168.1.101');
  const [conflictTargetIP, setConflictTargetIP] = useState<string>('192.168.1.64');
  const [conflictStartIP, setConflictStartIP] = useState<string>('192.168.1.101');
  const [conflictPass, setConflictPass] = useState<string>('');
  const [batchOutput, setBatchOutput] = useState<string>('');

  // Engineering Calculator State
  const [calcCams, setCalcCams] = useState<number>(8);
  const [calcRes, setCalcRes] = useState<number>(4);
  const [calcCodec, setCalcCodec] = useState<'h265plus' | 'h265' | 'h264'>('h265plus');
  const [calcFps, setCalcFps] = useState<number>(20);
  const [calcDays, setCalcDays] = useState<number>(30);
  const [calcModeFactor, setCalcModeFactor] = useState<number>(1.0);
  const [calcWatts, setCalcWatts] = useState<number>(6.5);
  const [calcCableLen, setCalcCableLen] = useState<number>(65);
  const [calcCableMat, setCalcCableMat] = useState<'cu' | 'cca'>('cu');
  const [calcFocal, setCalcFocal] = useState<number>(2.8);
  const [calcDist, setCalcDist] = useState<number>(10);

  // Event Log Console
  const [showLog, setShowLog] = useState<boolean>(false);
  const [logs, setLogs] = useState<string[]>([
    '[ProCam Radar v5.0] Нативное приложение запущено. Маршрутизация Кабель Ethernet + Wi-Fi (/32) активна.',
  ]);

  const appendLog = (msg: string) => {
    const ts = new Date().toLocaleTimeString('ru-RU');
    setLogs((prev) => [...prev.slice(-150), `[${ts}] ${msg}`]);
    setStatusMessage(msg.split('\n')[0]);
  };

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);

  const loadSysInfo = async () => {
    try {
      const r = await fetch('/api/info');
      const data: SysInfoResponse = await r.json();
      setSysInfo(data);
      if (data.subnet && !freeSubnet) {
        setFreeSubnet(data.subnet);
      }
    } catch {
      // ignore
    }
  };

  useEffect(() => {
    loadSysInfo();
    const timer = setInterval(loadSysInfo, 6000);
    return () => clearInterval(timer);
  }, []);

  // Select camera helper
  const handleSelectCamera = (cam: CameraEntry) => {
    setSelectedIP(cam.ip);
    setSelectedMAC(cam.mac || '');
    setSnapshotUrl('');
  };

  // Run Full Deduplicated Network Scan
  const handleScan = async () => {
    if (scanning) return;
    setScanning(true);
    appendLog(`Запуск двухуровневого сканирования сети (интерфейс: ${ifaceMode})...`);
    try {
      const r = await fetch(`/api/scan?iface=${encodeURIComponent(ifaceMode)}`);
      const raw: CameraEntry[] = (await r.json()) || [];
      const clean = deduplicateAndSortCameras(raw);
      setCameras(clean);
      await loadSysInfo();
      if (clean.length > 0) {
        const stillExists = clean.find(
          (c) => c.ip === selectedIP && (!selectedMAC || c.mac === selectedMAC)
        );
        if (!stillExists) {
          handleSelectCamera(clean[0]);
        }
      }
      const nvrCount = clean.filter((c) => c.device_type === 'nvr').length;
      const camCount = clean.length - nvrCount;
      appendLog(
        `Сканирование завершено: найдено ${clean.length} уникальных устройств (IP-камер: ${camCount}, NVR: ${nvrCount}). Дубликаты исключены.`
      );
    } catch (e) {
      appendLog(`Ошибка сканирования: ${e}`);
    } finally {
      setScanning(false);
    }
  };

  // Run Interactive Cable Plug Capture ("Unplug -> Plug")
  const handlePlugScan = async () => {
    if (plugScanning) return;
    setPlugScanning(true);
    setPlugCountdown(24);
    appendLog(
      'Поиск по кабелю («Отключи → Включи»): отключите патч-корд камеры на 3 сек и включите обратно...'
    );

    const iv = setInterval(() => {
      setPlugCountdown((prev) => (prev > 0 ? prev - 1 : 0));
    }, 1000);

    try {
      const r = await fetch(`/api/plugscan?iface=${encodeURIComponent(ifaceMode)}`);
      const raw: CameraEntry[] = (await r.json()) || [];
      const clean = deduplicateAndSortCameras(raw);
      setCameras(clean);
      if (clean.length > 0) {
        const caught = clean.find((c) => c.zone.includes('кабел')) || clean[0];
        handleSelectCamera(caught);
      }
      appendLog(`Поиск по кабелю завершен: в списке ${clean.length} устройств.`);
    } catch (e) {
      appendLog(`Ошибка перехвата по кабелю: ${e}`);
    } finally {
      clearInterval(iv);
      setPlugScanning(false);
    }
  };

  // Find Free IP Pool & Build 254 Matrix
  const handleFindFreeIPs = async (customCount?: number) => {
    const cnt = customCount ?? freeCount;
    if (customCount) setFreeCount(customCount);
    setFreeLoading(true);
    appendLog(`Проверка 254 адресов подсети ${freeSubnet || 'авто'} для подбора ${cnt} свободных IP...`);
    try {
      const r = await fetch(
        `/api/free?count=${encodeURIComponent(cnt)}&subnet=${encodeURIComponent(freeSubnet.trim())}`
      );
      const data: FreePoolResponse = await r.json();
      setFreePool(data);
      if (data.recommended_list && data.recommended_list.length > 0) {
        const first = data.recommended_list[0];
        setNewIP(first);
        setBatchStartIP(first);
        setConflictStartIP(first);
        appendLog(
          `Найдено ${data.total_free_in_subnet} свободных IP. Рекомендованный пул (${data.recommended_list.length} шт.): ${data.range_start} ➔ ${data.range_end}`
        );
      }
    } catch (e) {
      appendLog(`Ошибка подбора свободных IP: ${e}`);
    } finally {
      setFreeLoading(false);
    }
  };

  // Camera Actions
  const getTargetIface = (ip: string) => {
    const found = cameras.find((c) => c.ip === ip);
    return found?.iface || (ifaceMode !== 'auto' ? ifaceMode : '');
  };

  const handleOpenByMAC = async (ip = selectedIP, mac = selectedMAC) => {
    if (!ip) return;
    const targetIf = getTargetIface(ip);
    appendLog(`Привязка статического ARP (${ip} ➔ ${mac || 'авто'} на ${targetIf || 'авто'}) и открытие в браузере...`);
    try {
      const r = await fetch(
        `/api/isolate?ip=${encodeURIComponent(ip)}&mac=${encodeURIComponent(mac || '')}&iface=${encodeURIComponent(targetIf)}`
      );
      const txt = await r.text();
      appendLog(txt);
    } catch (e) {
      appendLog(`Ошибка открытия по MAC: ${e}`);
    }
  };

  const handleOpenWeb = async (ip = selectedIP) => {
    if (!ip) return;
    const targetIf = getTargetIface(ip);
    try {
      const r = await fetch(
        `/api/isolate?ip=${encodeURIComponent(ip)}&mac=&iface=${encodeURIComponent(targetIf)}`
      );
      const txt = await r.text();
      appendLog(txt);
    } catch (e) {
      appendLog(`Ошибка открытия браузера: ${e}`);
    }
  };

  const openExternalURL = async (url: string) => {
    try {
      await fetch(`/api/openurl?url=${encodeURIComponent(url)}`);
    } catch {
      window.open(url, '_blank', 'noopener,noreferrer');
    }
  };

  const handleCheckRTSP = async (
    ip = selectedIP,
    passOverride?: string,
    subOverride?: 'main' | 'sub',
    profOverride?: 'auto' | 'hikvision' | 'dahua' | 'onvif'
  ) => {
    if (!ip) return;
    const pass = (passOverride !== undefined ? passOverride : camPass).trim();
    const sub = subOverride || rtspStreamSub;
    const prof = profOverride || rtspProfile;
    const targetIf = getTargetIface(ip);
    setRtspChecking(true);
    try {
      const r = await fetch(
        `/api/rtsp/check?ip=${encodeURIComponent(ip)}&user=${encodeURIComponent(
          camUser || 'admin'
        )}&pass=${encodeURIComponent(pass)}&sub=${encodeURIComponent(
          sub
        )}&profile=${encodeURIComponent(prof)}&iface=${encodeURIComponent(targetIf)}`
      );
      if (r.ok) {
        const data = await r.json();
        setRtspStatus(data);
        if (data.message) appendLog(`[RTSP ${ip}] ${data.message}`);
      }
    } catch (e) {
      appendLog(`Ошибка проверки RTSP: ${e}`);
    } finally {
      setRtspChecking(false);
    }
  };

  const handlePlayRTSP = async (ip = selectedIP) => {
    if (!ip) return;
    setSelectedIP(ip);
    setMjpegError(false);
    setMjpegTick(Date.now());
    setRtspModalOpen(true);
    await handleCheckRTSP(ip);
  };

  const handleLaunchExternalRTSP = async (ip = selectedIP) => {
    if (!ip) return;
    const pass = camPass.trim() || '12345';
    const targetIf = getTargetIface(ip);
    appendLog(`Запуск внешнего плеера (FFplay / MPV / VLC) для камеры ${ip}...`);
    try {
      const r = await fetch(
        `/api/rtsp?ip=${encodeURIComponent(ip)}&user=${encodeURIComponent(
          camUser || 'admin'
        )}&pass=${encodeURIComponent(pass)}&sub=${encodeURIComponent(
          rtspStreamSub
        )}&profile=${encodeURIComponent(rtspProfile)}&iface=${encodeURIComponent(targetIf)}`
      );
      const txt = await r.text();
      appendLog(txt);
    } catch (e) {
      appendLog(`Ошибка запуска внешнего RTSP-плеера: ${e}`);
    }
  };

  const handleCaptureSnapshot = async (ip = selectedIP) => {
    if (!ip) return;
    const targetIf = getTargetIface(ip);
    setDiagLoading(true);
    appendLog(`Захват стоп-кадра JPEG с камеры ${ip}...`);
    try {
      const r = await fetch(
        `/api/snap?ip=${encodeURIComponent(ip)}&user=${encodeURIComponent(
          camUser || 'admin'
        )}&pass=${encodeURIComponent(camPass.trim())}&iface=${encodeURIComponent(targetIf)}`
      );
      const txt = await r.text();
      appendLog(txt);
      // Set image preview URL with cache-buster
      setSnapshotUrl(`/api/snap/image?ip=${encodeURIComponent(ip)}&t=${Date.now()}`);
    } catch (e) {
      appendLog(`Ошибка стоп-кадра: ${e}`);
    } finally {
      setDiagLoading(false);
    }
  };

  const handleInspectPassport = async (ip = selectedIP) => {
    if (!ip) return;
    const targetIf = getTargetIface(ip);
    setInspectorTab('passport');
    setDiagLoading(true);
    setPassportText('Беспарольный опрос паспорта камеры (ONVIF-часы, статус активации, порты, сертификат)...');
    try {
      const r = await fetch(
        `/api/caminfo?ip=${encodeURIComponent(ip)}&iface=${encodeURIComponent(targetIf)}`
      );
      const txt = await r.text();
      setPassportText(txt);
      appendLog(`Паспорт устройства ${ip} успешно получен.`);
    } catch (e) {
      setPassportText(`Ошибка опроса паспорта: ${e}`);
    } finally {
      setDiagLoading(false);
    }
  };

  const handleCableTest = async (ip = selectedIP) => {
    if (!ip) return;
    const targetIf = getTargetIface(ip);
    setInspectorTab('cable');
    setDiagLoading(true);
    setCableText('Тестирование физической витой пары RJ-45 пакетами 64B и 1472B (MTU 1500 DF)...');
    try {
      const r = await fetch(
        `/api/cable?ip=${encodeURIComponent(ip)}&iface=${encodeURIComponent(targetIf)}`
      );
      const txt = await r.text();
      setCableText(txt);

      // Parse packet loss percentages from output
      const lossMatches = Array.from(txt.matchAll(/(\d+(?:\.\d+)?)%\s*(?:packet\s*loss|потерь)/gi));
      const smallLoss = lossMatches[0] ? parseFloat(lossMatches[0][1]) : 0;
      const heavyLoss = lossMatches[1] ? parseFloat(lossMatches[1][1]) : smallLoss;
      let status: 'ok' | 'warning' | 'bad' = 'ok';
      let verdict = 'Линия RJ-45 / PoE в идеале: пакеты 64B и тяжелые видеокадры 1472B (MTU 1500) проходят без потерь.';
      if (heavyLoss > 0 && smallLoss === 0) {
        status = 'warning';
        verdict = `ВНИМАНИЕ: Короткие пинги (64B) проходят, но тяжелые кадры 1472B теряются (${heavyLoss}%)! Признак плохого обжима RJ-45, окисления контактов или превышения длины CCA-кабеля.`;
      } else if (heavyLoss > 20 || smallLoss > 10) {
        status = 'bad';
        verdict = `КРИТИЧЕСКИЕ ПОТЕРИ НА ЛИНИИ (64B: ${smallLoss}%, 1472B: ${heavyLoss}%)! Проверьте коннекторы RJ-45, блок питания PoE и целостность кабеля.`;
      }
      setCableStats({ smallLoss, heavyLoss, verdict, status });
      appendLog(`Тест кабеля ${ip} завершен (64B: ${smallLoss}% потерь, 1472B: ${heavyLoss}% потерь).`);
    } catch (e) {
      setCableText(`Ошибка проверки кабеля: ${e}`);
    } finally {
      setDiagLoading(false);
    }
  };

  const handleChangeIP = async () => {
    if (!selectedIP || !newIP.trim()) {
      appendLog('Укажите текущий и новый IP-адрес камеры.');
      return;
    }
    const targetIf = getTargetIface(selectedIP);
    setDiagLoading(true);
    appendLog(`Смена IP-адреса камеры ${selectedIP} ➔ ${newIP.trim()} (адаптер: ${targetIf || 'авто'})...`);
    try {
      const r = await fetch(
        `/api/setip?old=${encodeURIComponent(selectedIP)}&new=${encodeURIComponent(
          newIP.trim()
        )}&mac=${encodeURIComponent(selectedMAC)}&user=${encodeURIComponent(
          camUser || 'admin'
        )}&pass=${encodeURIComponent(camPass.trim())}&iface=${encodeURIComponent(targetIf)}`
      );
      const txt = await r.text();
      appendLog(txt);
      setPassportText(txt);
    } catch (e) {
      appendLog(`Ошибка смены IP: ${e}`);
    } finally {
      setDiagLoading(false);
    }
  };

  const handleCleanSubnets = async () => {
    try {
      const r = await fetch('/api/clean');
      const txt = await r.text();
      appendLog(txt);
      await loadSysInfo();
    } catch (e) {
      appendLog(`Ошибка очистки подсетей: ${e}`);
    }
  };

  const handleDownloadPDF = async () => {
    appendLog('Генерация инженерного PDF-паспорта объекта...');
    try {
      const r = await fetch('/api/pdf/open');
      if (r.ok) {
        const data = await r.json();
        appendLog(`PDF-отчет сохранен и открыт: ${data.path || 'ProCam_Radar_Audit.pdf'}`);
        return;
      }
    } catch {
      // fallback
    }
    window.open('/api/pdf', '_blank');
  };

  const handleBatchActivate = async () => {
    if (!batchPass.trim()) {
      appendLog('Введите новый пароль (минимум 8 символов) для пакетной активации.');
      return;
    }
    setBatchOutput('Выполняется пакетная активация камер на заводских IP...');
    try {
      const r = await fetch(
        `/api/activate?pass=${encodeURIComponent(batchPass.trim())}&start=${encodeURIComponent(
          batchStartIP.trim()
        )}`
      );
      const txt = await r.text();
      setBatchOutput(txt);
      appendLog('Пакетная активация завершена.');
    } catch (e) {
      setBatchOutput(`Ошибка: ${e}`);
    }
  };

  const handleBatchResolve = async () => {
    if (!conflictTargetIP.trim() || !conflictStartIP.trim()) {
      appendLog('Укажите конфликтный IP и стартовый свободный IP.');
      return;
    }
    setBatchOutput(
      `Изоляция камер по MAC на адресе ${conflictTargetIP} и разведение на ${conflictStartIP}, +1, +2...`
    );
    try {
      const r = await fetch(
        `/api/resolve?conflict=${encodeURIComponent(
          conflictTargetIP.trim()
        )}&start=${encodeURIComponent(conflictStartIP.trim())}&pass=${encodeURIComponent(
          conflictPass.trim()
        )}`
      );
      const txt = await r.text();
      setBatchOutput(txt);
      appendLog('Автоматическое разведение конфликта завершено.');
    } catch (e) {
      setBatchOutput(`Ошибка: ${e}`);
    }
  };

  const copyText = (txt: string, label = 'Скопировано') => {
    if (!txt) return;
    navigator.clipboard?.writeText(txt);
    appendLog(`${label}: ${txt}`);
  };

  // Counts & Filtered Table Data
  const counts = useMemo(() => {
    let cam = 0,
      nvr = 0,
      local = 0,
      foreign = 0,
      conflict = 0;
    for (const c of cameras) {
      if (c.device_type === 'nvr') nvr++;
      else cam++;
      if (c.zone_type === 'conflict' || c.conflict) conflict++;
      else if (c.zone_type === 'foreign') foreign++;
      else local++;
    }
    return { all: cameras.length, cam, nvr, local, foreign, conflict };
  }, [cameras]);

  const filteredCameras = useMemo(() => {
    const q = searchQuery.toLowerCase().trim();
    const list = cameras.filter((c) => {
      if (filterMode === 'camera' && c.device_type === 'nvr') return false;
      if (filterMode === 'nvr' && c.device_type !== 'nvr') return false;
      if (filterMode === 'local' && c.zone_type !== 'local') return false;
      if (filterMode === 'foreign' && c.zone_type !== 'foreign') return false;
      if (filterMode === 'conflict' && c.zone_type !== 'conflict' && !c.conflict) return false;
      if (!q) return true;
      const hay = [
        c.ip,
        c.mac,
        c.vendor,
        c.model,
        c.serial,
        c.device_label,
        c.iface,
        c.subnet_alias,
        (c.ports || []).join(' '),
      ]
        .join(' ')
        .toLowerCase();
      return hay.includes(q);
    });

    list.sort((a, b) => {
      let cmp = 0;
      if (sortBy === 'ip') cmp = ipToNumber(a.ip) - ipToNumber(b.ip);
      else if (sortBy === 'vendor') cmp = (a.vendor || '').localeCompare(b.vendor || '');
      else if (sortBy === 'model') cmp = (a.model || '').localeCompare(b.model || '');
      else if (sortBy === 'type') cmp = (a.device_type || '').localeCompare(b.device_type || '');
      else if (sortBy === 'zone') cmp = (a.zone || '').localeCompare(b.zone || '');
      if (cmp === 0) cmp = ipToNumber(a.ip) - ipToNumber(b.ip);
      return sortAsc ? cmp : -cmp;
    });

    return list;
  }, [cameras, filterMode, searchQuery, sortBy, sortAsc]);

  const selectedCameraObj = useMemo(() => {
    return (
      cameras.find((c) => c.ip === selectedIP && (!selectedMAC || c.mac === selectedMAC)) ||
      cameras.find((c) => c.ip === selectedIP) ||
      null
    );
  }, [cameras, selectedIP, selectedMAC]);

  // 254 Subnet Host Grid Builder
  const subnetMatrix = useMemo<HostCell[]>(() => {
    if (freePool?.host_map && freePool.host_map.length > 0) {
      return freePool.host_map;
    }
    const baseSub = freePool?.subnet || freeSubnet || sysInfo?.subnet || '192.168.1.0/24';
    const prefix = baseSub.split('/')[0].split('.').slice(0, 3).join('.') || '192.168.1';
    const recSet = new Set(freePool?.recommended_list || []);
    const camMap = new Map<string, CameraEntry>();
    for (const c of cameras) camMap.set(c.ip, c);
    const myIP = (sysInfo?.ip || '').split('/')[0];

    const cells: HostCell[] = [];
    for (let h = 1; h <= 254; h++) {
      const ip = `${prefix}.${h}`;
      const cam = camMap.get(ip);
      let status: HostCell['status'] = 'free';
      let label = 'Свободен';
      if (h === 1) {
        status = 'gateway';
        label = 'Шлюз роутера (.1)';
      } else if (ip === myIP || h >= 248) {
        status = 'self';
        label = ip === myIP ? 'Этот ПК' : 'Служебный мост (.248-.254)';
      } else if (cam) {
        status = cam.device_type === 'nvr' ? 'nvr' : 'camera';
        label = `${cam.device_label}: ${cam.vendor} (${cam.mac || '-'})`;
      } else if (recSet.has(ip)) {
        status = 'recommended';
        label = 'Рекомендован для новой камеры';
      }
      cells.push({ host: h, ip, status, mac: cam?.mac, label });
    }
    return cells;
  }, [freePool, freeSubnet, sysInfo, cameras]);

  // Engineering Calculator Computed Results
  const calcResults = useMemo(() => {
    const baseMbpsMap: Record<number, number> = { 2: 2.2, 4: 3.8, 5: 4.5, 6: 5.2, 8: 7.5 };
    let mbpsPerCam = baseMbpsMap[calcRes] || 3.8;
    if (calcCodec === 'h265plus') mbpsPerCam *= 0.52;
    else if (calcCodec === 'h264') mbpsPerCam *= 1.85;
    mbpsPerCam = Math.round(mbpsPerCam * (calcFps / 25.0) * 100) / 100;

    const totalMbps = Math.round(mbpsPerCam * calcCams * 10) / 10;
    const gbPerCamDay = ((mbpsPerCam * 3600 * 24) / 8 / 1024) * calcModeFactor;
    const totalGBPerDay = gbPerCamDay * calcCams;
    const rawTotalTB = (totalGBPerDay * calcDays) / 1024;
    const diskNominalTB = rawTotalTB / 0.909;

    const stdSizes = [1, 2, 4, 6, 8, 10, 12, 16, 18, 20];
    const single = stdSizes.find((s) => s >= diskNominalTB);
    let recDisk = '1× 1 ТБ';
    if (single) {
      recDisk = `1× ${single} ТБ (WD Purple / SkyHawk)`;
    } else {
      const bays = Math.ceil(diskNominalTB / 10);
      const perBay = stdSizes.find((s) => s * bays >= diskNominalTB) || 10;
      recDisk = `${bays}× ${perBay} ТБ (${bays * perBay} ТБ суммарно)`;
    }

    const totalPoEWatts = Math.round(calcCams * calcWatts);
    const recSwitchWatts = Math.ceil((totalPoEWatts * 1.3) / 10) * 10;
    const nvrChannels =
      calcCams > 32 ? 64 : calcCams > 16 ? 32 : calcCams > 8 ? 16 : calcCams > 4 ? 8 : 4;
    const recNvrBandwidth = Math.max(40, Math.ceil((totalMbps * 1.5) / 40) * 40);

    const loopOhmPer100m = calcCableMat === 'cu' ? 18.8 : 29.5;
    const loopR = (loopOhmPer100m * calcCableLen) / 100.0;
    const currentA = calcWatts / 48.0;
    const vDrop = Math.round(currentA * loopR * 10) / 10;
    const vAtCam = Math.round((48.0 - vDrop) * 10) / 10;

    const fovMap: Record<number, number> = { 2.8: 103, 4.0: 83, 6.0: 54, 12.0: 25 };
    const angleDeg = fovMap[calcFocal] || 90;
    const sceneWidthM = Math.round(2 * calcDist * Math.tan((angleDeg * Math.PI) / 360) * 10) / 10;
    const horizPxMap: Record<number, number> = { 2: 1920, 4: 2560, 5: 2880, 6: 3200, 8: 3840 };
    const hPx = horizPxMap[calcRes] || 2560;
    const ppm = Math.round(hPx / Math.max(0.5, sceneWidthM));
    const factorW = 2 * Math.tan((angleDeg * Math.PI) / 360);
    const doriId = Math.round((hPx / 250 / factorW) * 10) / 10;
    const doriRec = Math.round((hPx / 125 / factorW) * 10) / 10;
    const doriDet = Math.round((hPx / 25 / factorW) * 10) / 10;

    return {
      mbpsPerCam,
      totalMbps,
      gbPerCamDay,
      totalGBPerDay,
      rawTotalTB,
      recDisk,
      totalPoEWatts,
      recSwitchWatts,
      nvrChannels,
      recNvrBandwidth,
      vAtCam,
      vDrop,
      angleDeg,
      sceneWidthM,
      ppm,
      doriId,
      doriRec,
      doriDet,
    };
  }, [
    calcCams,
    calcRes,
    calcCodec,
    calcFps,
    calcDays,
    calcModeFactor,
    calcWatts,
    calcCableLen,
    calcCableMat,
    calcFocal,
    calcDist,
  ]);

  const toggleSort = (col: typeof sortBy) => {
    if (sortBy === col) setSortAsc(!sortAsc);
    else {
      setSortBy(col);
      setSortAsc(true);
    }
  };

  const handleExportCSV = () => {
    if (cameras.length === 0) return;
    const header = 'IP;MAC;Type;Vendor;Model;Serial;Ports;Zone;Interface\n';
    const rows = cameras
      .map((c) =>
        [
          c.ip,
          c.mac,
          c.device_label,
          c.vendor,
          c.model,
          c.serial,
          (c.ports || []).join(','),
          c.zone,
          c.iface,
        ].join(';')
      )
      .join('\n');
    const blob = new Blob(['\uFEFF' + header + rows], { type: 'text/csv;charset=utf-8;' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `ProCam_Radar_Export_${new Date().toISOString().slice(0, 10)}.csv`;
    a.click();
    URL.revokeObjectURL(url);
    appendLog(`Экспортировано ${cameras.length} устройств в CSV.`);
  };

  return (
    <div className="app-shell">
      {/* LEFT DESKTOP NAVIGATION SIDEBAR */}
      <aside className="sidebar">
        <div>
          <div className="brand-header">
            <svg width="38" height="38" viewBox="0 0 64 64" fill="none">
              <rect width="64" height="64" rx="14" fill="#09090B" stroke="#272730" strokeWidth="2" />
              <circle cx="32" cy="32" r="22" fill="#0E131F" stroke="#10B981" strokeWidth="1.5" strokeOpacity="0.4" />
              <circle cx="32" cy="32" r="14" stroke="#10B981" strokeWidth="1.2" strokeDasharray="3 2" strokeOpacity="0.5" />
              <circle cx="32" cy="32" r="6" stroke="#10B981" strokeWidth="1.2" strokeOpacity="0.7" />
              <line x1="10" y1="32" x2="54" y2="32" stroke="#272730" strokeWidth="1.2" />
              <line x1="32" y1="10" x2="32" y2="54" stroke="#272730" strokeWidth="1.2" />
              <line x1="32" y1="32" x2="47" y2="17" stroke="#10B981" strokeWidth="2.4" strokeLinecap="round" />
              <circle cx="41" cy="23" r="2.5" fill="#34D399" />
              <circle cx="24" cy="24" r="1.8" fill="#10B981" />
              <circle cx="32" cy="32" r="3" fill="#10B981" />
            </svg>
            <div>
              <div className="brand-title">PROCAM RADAR</div>
              <div className="brand-sub">Desktop Suite v5.0</div>
            </div>
          </div>

          <div className="nav-list">
            <button
              className={`nav-item ${navTab === 'radar' ? 'active' : ''}`}
              onClick={() => setNavTab('radar')}
            >
              <span>📡 Радар и Устройства</span>
              <span className="nav-badge mono">{counts.all}</span>
            </button>

            <button
              className={`nav-item ${navTab === 'cable' ? 'active' : ''}`}
              onClick={() => setNavTab('cable')}
            >
              <span>🔌 Поиск по кабелю</span>
              {plugScanning && <span className="nav-badge mono">{plugCountdown}с</span>}
            </button>

            <button
              className={`nav-item ${navTab === 'ipmap' ? 'active' : ''}`}
              onClick={() => {
                setNavTab('ipmap');
                if (!freePool) handleFindFreeIPs();
              }}
            >
              <span>🗺️ Карта подсети (254 IP)</span>
            </button>

            <button
              className={`nav-item ${navTab === 'batch' ? 'active' : ''}`}
              onClick={() => setNavTab('batch')}
            >
              <span>⚡ Пакетная настройка</span>
              {counts.conflict > 0 && (
                <span
                  className="nav-badge mono"
                  style={{ background: 'var(--red-soft)', color: 'var(--red)' }}
                >
                  {counts.conflict}
                </span>
              )}
            </button>

            <button
              className={`nav-item ${navTab === 'calc' ? 'active' : ''}`}
              onClick={() => setNavTab('calc')}
            >
              <span>🧮 Инженерный расчет</span>
            </button>

            <button
              className={`nav-item ${navTab === 'help' ? 'active' : ''}`}
              onClick={() => setNavTab('help')}
            >
              <span>📘 Отчет PDF и Справка</span>
            </button>

            <button
              className={`nav-item ${navTab === 'about' ? 'active' : ''}`}
              onClick={() => setNavTab('about')}
            >
              <span>👤 Обо мне и Проекты</span>
              <span className="nav-badge mono" style={{ color: 'var(--emerald)' }}>
                YEEE.KZ
              </span>
            </button>
          </div>
        </div>

        {/* Sidebar Footer Controls */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
          <div
            style={{
              background: 'var(--bg-surface)',
              border: '1px solid var(--border)',
              borderRadius: '9px',
              padding: '10px',
              fontSize: '11px',
              color: 'var(--text-secondary)',
            }}
          >
            <div style={{ fontWeight: 800, color: 'var(--text-primary)', marginBottom: '4px' }}>
              Мульти-сеть (2×LAN + 2×Wi-Fi + VPN)
            </div>
            <div>
              Привязка сокетов к каждому адаптеру и точечные маршруты /32 без потери интернета и Tailscale.
            </div>
          </div>

          <div style={{ display: 'flex', gap: '6px' }}>
            <button
              className="btn"
              style={{ flex: 1 }}
              onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
            >
              {theme === 'dark' ? '☀️ Светлая тема' : '🌙 Темная тема'}
            </button>
            <button
              className="btn"
              title="Очистить временные сетевые мосты"
              onClick={handleCleanSubnets}
            >
              🧹
            </button>
          </div>
        </div>
      </aside>

      {/* MAIN WORKSPACE COLUMN */}
      <div className="workspace-col">
        {/* TOP NATIVE BAR */}
        <header className="topbar">
          <div className="net-pills">
            {sysInfo?.interfaces && sysInfo.interfaces.length > 0 ? (
              sysInfo.interfaces.map((inf: NetInterfaceInfo) => {
                const isSelected = ifaceMode === inf.name || ifaceMode === inf.kind;
                return (
                  <span
                    key={inf.name}
                    className={`net-pill mono ${inf.carrier ? 'online' : ''}`}
                    style={{
                      cursor: 'pointer',
                      borderColor: isSelected ? 'var(--emerald)' : undefined,
                      background: isSelected ? 'var(--emerald-soft)' : undefined,
                    }}
                    onClick={() =>
                      setIfaceMode((prev) => (prev === inf.name ? 'auto' : inf.name))
                    }
                    title={`Кликните для фильтрации по ${inf.name} (${inf.subnet || 'L2 / VPN'})`}
                  >
                    <span className={`dot ${inf.carrier ? 'online' : ''}`} />
                    {inf.label}
                  </span>
                );
              })
            ) : (
              <span className="net-pill mono online">
                <span className="dot online" />
                Сеть активна ({sysInfo?.ip || 'L2/L3'})
              </span>
            )}
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <select
              className="select"
              value={ifaceMode}
              onChange={(e) => setIfaceMode(e.target.value)}
              title="Выбор сетевого интерфейса (или группы адаптеров) для сканирования"
            >
              <option value="auto">Все сети (2×Кабель + 2×Wi-Fi + VPN)</option>
              <option value="ethernet">Все кабели Ethernet (LAN 1 + LAN 2)</option>
              <option value="wifi">Все адаптеры Wi-Fi (Wi-Fi 1 + Wi-Fi 2)</option>
              <option value="vpn">Только VPN / Tailscale / WireGuard</option>
              {sysInfo?.interfaces?.map((inf) => (
                <option key={inf.name} value={inf.name}>
                  Адаптер: {inf.label}
                </option>
              ))}
            </select>

            <button className="btn btn-primary" disabled={scanning} onClick={handleScan}>
              <span>{scanning ? '⏳' : '📡'}</span>
              <span>{scanning ? 'Сканирование...' : 'Найти камеры и NVR'}</span>
            </button>

            <button className="btn" onClick={handleDownloadPDF} title="Сформировать PDF-паспорт">
              📄 PDF-отчет
            </button>
          </div>
        </header>

        {/* WORKSPACE CONTENT */}
        <div className="content-area">
          {/* TAB 1: RADAR & DEVICES TABLE */}
          {navTab === 'radar' && (
            <>
              <div className="main-pane">
                {/* Filter & Search Bar */}
                <div className="filter-bar">
                  <div className="filter-tabs">
                    <button
                      className={`filter-tab ${filterMode === 'all' ? 'active' : ''}`}
                      onClick={() => setFilterMode('all')}
                    >
                      Все <span className="mono">({counts.all})</span>
                    </button>
                    <button
                      className={`filter-tab ${filterMode === 'camera' ? 'active' : ''}`}
                      onClick={() => setFilterMode('camera')}
                    >
                      IP-Камеры <span className="mono">({counts.cam})</span>
                    </button>
                    <button
                      className={`filter-tab ${filterMode === 'nvr' ? 'active' : ''}`}
                      onClick={() => setFilterMode('nvr')}
                    >
                      NVR / Регистраторы <span className="mono">({counts.nvr})</span>
                    </button>
                    <button
                      className={`filter-tab ${filterMode === 'local' ? 'active' : ''}`}
                      onClick={() => setFilterMode('local')}
                    >
                      Своя сеть <span className="mono">({counts.local})</span>
                    </button>
                    <button
                      className={`filter-tab ${filterMode === 'foreign' ? 'active' : ''}`}
                      onClick={() => setFilterMode('foreign')}
                    >
                      Чужая подсеть <span className="mono">({counts.foreign})</span>
                    </button>
                    <button
                      className={`filter-tab ${filterMode === 'conflict' ? 'active' : ''}`}
                      onClick={() => setFilterMode('conflict')}
                    >
                      Конфликты IP <span className="mono">({counts.conflict})</span>
                    </button>
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                    <input
                      type="text"
                      className="input"
                      placeholder="Поиск по IP, MAC, модели, порту..."
                      style={{ width: '240px' }}
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                    />
                    {cameras.length > 0 && (
                      <button className="btn btn-sm" onClick={handleExportCSV}>
                        📥 CSV
                      </button>
                    )}
                  </div>
                </div>

                {/* Prominent Camera Credentials Bar (RTSP / ISAPI / Snapshot) */}
                <div className="cred-bar">
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                    <span
                      style={{
                        fontWeight: 800,
                        color: 'var(--emerald)',
                        fontSize: '12px',
                        display: 'inline-flex',
                        alignItems: 'center',
                        gap: '5px',
                      }}
                    >
                      🔑 Пароль камер (для RTSP-видео и смены IP):
                    </span>
                    <input
                      type="text"
                      className="input mono"
                      style={{ width: '92px', padding: '5px 8px', fontSize: '12px' }}
                      placeholder="Логин"
                      value={camUser}
                      onChange={(e) => setCamUser(e.target.value)}
                      title="Логин администратора камер (обычно admin)"
                    />
                    <div style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
                      <input
                        type={showPass ? 'text' : 'password'}
                        className="input mono"
                        style={{
                          width: '215px',
                          padding: '5px 9px',
                          fontSize: '12px',
                          borderColor: camPass ? 'var(--emerald)' : undefined,
                        }}
                        placeholder="Введите пароль от камер..."
                        value={camPass}
                        onChange={(e) => setCamPass(e.target.value)}
                      />
                      <button
                        type="button"
                        className="btn btn-sm"
                        onClick={() => setShowPass(!showPass)}
                        title={showPass ? 'Скрыть пароль' : 'Показать пароль'}
                      >
                        {showPass ? '🙈' : '👁️'}
                      </button>
                    </div>
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '5px', flexWrap: 'wrap' }}>
                    <span style={{ fontSize: '11px', color: 'var(--text-muted)' }}>Частые пароли:</span>
                    {['12345', 'admin', '123456', 'admin12345'].map((preset) => (
                      <button
                        key={preset}
                        type="button"
                        className={`preset-chip mono ${camPass === preset ? 'active' : ''}`}
                        onClick={() => setCamPass(preset)}
                      >
                        {preset}
                      </button>
                    ))}
                    <button
                      type="button"
                      className="btn btn-sm btn-primary"
                      onClick={() => handlePlayRTSP(selectedIP)}
                      title="Открыть встроенный RTSP-видеоплеер для выбранной камеры"
                    >
                      🎥 Смотреть видео ({selectedIP})
                    </button>
                  </div>
                </div>

                {/* Deduplicated Camera Table */}
                <div className="table-card">
                  <table className="radar-table">
                    <thead>
                      <tr>
                        <th style={{ width: '42px' }}>#</th>
                        <th onClick={() => toggleSort('zone')}>
                          Статус {sortBy === 'zone' ? (sortAsc ? '↑' : '↓') : ''}
                        </th>
                        <th onClick={() => toggleSort('type')}>
                          Тип {sortBy === 'type' ? (sortAsc ? '↑' : '↓') : ''}
                        </th>
                        <th onClick={() => toggleSort('ip')}>
                          IP-адрес {sortBy === 'ip' ? (sortAsc ? '↑' : '↓') : ''}
                        </th>
                        <th>MAC-адрес</th>
                        <th onClick={() => toggleSort('vendor')}>
                          Производитель {sortBy === 'vendor' ? (sortAsc ? '↑' : '↓') : ''}
                        </th>
                        <th onClick={() => toggleSort('model')}>
                          Модель / Серийный № {sortBy === 'model' ? (sortAsc ? '↑' : '↓') : ''}
                        </th>
                        <th>Порты</th>
                        <th>Адаптер / Мост</th>
                        <th style={{ textAlign: 'right' }}>Действия</th>
                      </tr>
                    </thead>
                    <tbody>
                      {filteredCameras.length === 0 ? (
                        <tr>
                          <td
                            colSpan={10}
                            style={{
                              textAlign: 'center',
                              padding: '56px 20px',
                              color: 'var(--text-secondary)',
                            }}
                          >
                            <div
                              style={{
                                fontSize: '15px',
                                fontWeight: 800,
                                color: 'var(--text-primary)',
                                marginBottom: '6px',
                              }}
                            >
                              {scanning
                                ? 'Идет опрос ONVIF, Hikvision SADP, Dahua DHIP и ARP-таблицы...'
                                : 'Нажмите «Найти камеры и NVR» для запуска сканирования'}
                            </div>
                            <div style={{ marginBottom: '16px', fontSize: '12.5px' }}>
                              Каждое устройство проходит строгую дедупликацию по IP и физическому MAC-адресу.
                            </div>
                            <div style={{ display: 'inline-flex', gap: '8px' }}>
                              <button
                                className="btn btn-primary"
                                disabled={scanning}
                                onClick={handleScan}
                              >
                                📡 Найти камеры и NVR
                              </button>
                              <button
                                className="btn"
                                onClick={() => {
                                  setNavTab('ipmap');
                                  handleFindFreeIPs();
                                }}
                              >
                                🗺️ Карта свободных IP
                              </button>
                            </div>
                          </td>
                        </tr>
                      ) : (
                        filteredCameras.map((cam, idx) => {
                          const isSelected =
                            cam.ip === selectedIP && (!selectedMAC || cam.mac === selectedMAC);
                          const isNVR = cam.device_type === 'nvr';
                          const zoneBadgeClass =
                            cam.zone_type === 'conflict' || cam.conflict
                              ? 'badge-conflict'
                              : cam.zone_type === 'foreign'
                              ? 'badge-foreign'
                              : 'badge-local';

                          return (
                            <tr
                              key={`${cam.ip}|${cam.mac || idx}`}
                              className={isSelected ? 'selected' : ''}
                              onClick={() => handleSelectCamera(cam)}
                            >
                              <td className="mono" style={{ color: 'var(--text-muted)' }}>
                                {idx + 1}
                              </td>
                              <td>
                                <span className={`badge ${zoneBadgeClass}`}>{cam.zone}</span>
                              </td>
                              <td>
                                <span className={`badge ${isNVR ? 'badge-nvr' : 'badge-camera'}`}>
                                  {isNVR ? 'NVR' : 'КАМЕРА'}
                                </span>
                              </td>
                              <td
                                className="mono"
                                style={{ fontWeight: 800, color: 'var(--text-primary)' }}
                                onClick={(e) => {
                                  e.stopPropagation();
                                  handleSelectCamera(cam);
                                  copyText(cam.ip, 'Скопирован IP');
                                }}
                                title="Кликните для копирования IP"
                              >
                                {cam.ip}
                              </td>
                              <td
                                className="mono"
                                style={{ color: 'var(--text-secondary)', fontSize: '11.5px' }}
                                onClick={(e) => {
                                  e.stopPropagation();
                                  handleSelectCamera(cam);
                                  if (cam.mac) copyText(cam.mac, 'Скопирован MAC');
                                }}
                                title="Кликните для копирования MAC"
                              >
                                {cam.mac || '—'}
                              </td>
                              <td style={{ fontWeight: 700 }}>{cam.vendor || 'ONVIF'}</td>
                              <td style={{ color: 'var(--text-secondary)' }}>
                                <span style={{ color: 'var(--text-primary)', fontWeight: 600 }}>
                                  {cam.model || 'IP-Камера'}
                                </span>
                                {cam.serial && cam.serial !== 'Н/Д' ? (
                                  <span
                                    className="mono"
                                    style={{
                                      marginLeft: '6px',
                                      fontSize: '11px',
                                      color: 'var(--text-muted)',
                                    }}
                                  >
                                    · {cam.serial}
                                  </span>
                                ) : null}
                              </td>
                              <td className="mono">
                                {(cam.ports || []).length > 0 ? (
                                  cam.ports.map((p) => (
                                    <span key={p} className="port-tag">
                                      {p}
                                    </span>
                                  ))
                                ) : (
                                  <span className="port-tag">L2</span>
                                )}
                              </td>
                              <td style={{ fontSize: '11.5px', color: 'var(--text-secondary)' }}>
                                <span className="mono" style={{ fontWeight: 700 }}>
                                  {cam.iface || 'LAN'}
                                </span>{' '}
                                · {cam.subnet_alias}
                              </td>
                              <td style={{ textAlign: 'right' }}>
                                <div style={{ display: 'inline-flex', gap: '4px' }}>
                                  <button
                                    className="btn btn-sm"
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      handleSelectCamera(cam);
                                      handleOpenWeb(cam.ip);
                                    }}
                                  >
                                    Веб
                                  </button>
                                  <button
                                    className="btn btn-sm"
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      handleSelectCamera(cam);
                                      handleOpenByMAC(cam.ip, cam.mac);
                                    }}
                                    title="Привязать статический ARP по MAC и открыть"
                                  >
                                    По MAC
                                  </button>
                                  <button
                                    className="btn btn-sm"
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      handleSelectCamera(cam);
                                      handlePlayRTSP(cam.ip);
                                    }}
                                  >
                                    RTSP
                                  </button>
                                </div>
                              </td>
                            </tr>
                          );
                        })
                      )}
                    </tbody>
                  </table>
                </div>
              </div>

              {/* RIGHT DEVICE INSPECTOR & WORKBENCH DRAWER */}
              <aside className="inspector-pane">
                <div className="inspector-header">
                  <div
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      marginBottom: '6px',
                    }}
                  >
                    <span className="section-title">// ИНСПЕКТОР УСТРОЙСТВА</span>
                    {selectedCameraObj && (
                      <span
                        className={`badge ${
                          selectedCameraObj.device_type === 'nvr' ? 'badge-nvr' : 'badge-camera'
                        }`}
                      >
                        {selectedCameraObj.device_label}
                      </span>
                    )}
                  </div>
                  <div
                    className="mono"
                    style={{
                      fontSize: '17px',
                      fontWeight: 900,
                      color: 'var(--text-primary)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                    }}
                  >
                    <span>{selectedIP || 'Не выбрана'}</span>
                    <span
                      style={{
                        fontSize: '12px',
                        fontWeight: 600,
                        color: 'var(--text-secondary)',
                      }}
                    >
                      {selectedMAC || 'MAC авто'}
                    </span>
                  </div>
                  {selectedCameraObj && (
                    <div
                      style={{
                        fontSize: '12px',
                        color: 'var(--text-secondary)',
                        marginTop: '3px',
                      }}
                    >
                      {selectedCameraObj.vendor} · {selectedCameraObj.model}
                    </div>
                  )}
                </div>

                {/* Inspector Tabs */}
                <div className="inspector-tabs">
                  <button
                    className={`inspector-tab ${inspectorTab === 'control' ? 'active' : ''}`}
                    onClick={() => setInspectorTab('control')}
                  >
                    Управление / IP
                  </button>
                  <button
                    className={`inspector-tab ${inspectorTab === 'passport' ? 'active' : ''}`}
                    onClick={() => {
                      setInspectorTab('passport');
                      if (!passportText && selectedIP) handleInspectPassport(selectedIP);
                    }}
                  >
                    Паспорт ONVIF
                  </button>
                  <button
                    className={`inspector-tab ${inspectorTab === 'cable' ? 'active' : ''}`}
                    onClick={() => {
                      setInspectorTab('cable');
                      if (!cableText && selectedIP) handleCableTest(selectedIP);
                    }}
                  >
                    Тест кабеля
                  </button>
                </div>

                <div className="inspector-body">
                  {/* INSPECTOR TAB 1: CONTROL & IP CHANGE */}
                  {inspectorTab === 'control' && (
                    <>
                      <div className="info-grid">
                        <div className="info-cell">
                          <div className="info-label">Текущий IP</div>
                          <input
                            className="input mono"
                            style={{ width: '100%', fontWeight: 800, padding: '5px 8px' }}
                            value={selectedIP}
                            onChange={(e) => setSelectedIP(e.target.value)}
                          />
                        </div>
                        <div className="info-cell">
                          <div className="info-label">MAC-адрес</div>
                          <input
                            className="input mono"
                            style={{ width: '100%', padding: '5px 8px' }}
                            placeholder="AA:BB:CC:DD:EE:FF"
                            value={selectedMAC}
                            onChange={(e) => setSelectedMAC(e.target.value)}
                          />
                        </div>
                      </div>

                      <div className="section-box">
                        <div className="section-title">Быстрый доступ и диагностика</div>
                        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px' }}>
                          <button
                            className="btn btn-primary"
                            style={{ gridColumn: '1 / -1' }}
                            onClick={() => handleOpenByMAC(selectedIP, selectedMAC)}
                          >
                            🔓 Открыть по MAC (при конфликте)
                          </button>
                          <button className="btn" onClick={() => handleOpenWeb(selectedIP)}>
                            🌐 Открыть в браузере
                          </button>
                          <button className="btn" onClick={() => handlePlayRTSP(selectedIP)}>
                            ▶️ Видео RTSP
                          </button>
                          <button
                            className="btn"
                            disabled={diagLoading}
                            onClick={() => handleCaptureSnapshot(selectedIP)}
                          >
                            📸 Стоп-кадр JPEG
                          </button>
                          <button
                            className="btn"
                            disabled={diagLoading}
                            onClick={() => handleInspectPassport(selectedIP)}
                          >
                            🪪 Паспорт камеры
                          </button>
                          <button
                            className="btn"
                            style={{ gridColumn: '1 / -1' }}
                            disabled={diagLoading}
                            onClick={() => handleCableTest(selectedIP)}
                          >
                            🔌 Стресс-тест кабеля RJ-45 и PoE (64B vs 1472B)
                          </button>
                        </div>
                      </div>

                      {/* Live Snapshot Preview */}
                      {snapshotUrl && (
                        <div className="section-box">
                          <div className="section-title">Контрольный стоп-кадр ({selectedIP})</div>
                          <img
                            src={snapshotUrl}
                            alt="Camera Snapshot"
                            style={{
                              width: '100%',
                              borderRadius: '8px',
                              border: '1px solid var(--border)',
                              background: '#000',
                            }}
                            onError={() => setSnapshotUrl('')}
                          />
                        </div>
                      )}

                      {/* Change IP Form */}
                      <div className="section-box">
                        <div
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'space-between',
                          }}
                        >
                          <span className="section-title">Смена IP-адреса камеры</span>
                          <button
                            className="btn btn-sm"
                            onClick={() => handleFindFreeIPs(8)}
                            title="Автоматически найти свободный IP в подсети"
                          >
                            🎯 Авто-подбор IP
                          </button>
                        </div>

                        <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                          <div>
                            <div className="info-label">Новый свободный IP-адрес</div>
                            <input
                              className="input mono"
                              style={{ width: '100%', fontWeight: 800 }}
                              placeholder="192.168.1.101"
                              value={newIP}
                              onChange={(e) => setNewIP(e.target.value)}
                            />
                          </div>

                          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '6px' }}>
                            <div>
                              <div className="info-label">Логин</div>
                              <input
                                className="input mono"
                                style={{ width: '100%' }}
                                value={camUser}
                                onChange={(e) => setCamUser(e.target.value)}
                              />
                            </div>
                            <div>
                              <div className="info-label">Пароль камеры</div>
                              <input
                                type="password"
                                className="input"
                                style={{ width: '100%' }}
                                placeholder="Авто-подбор"
                                value={camPass}
                                onChange={(e) => setCamPass(e.target.value)}
                              />
                            </div>
                          </div>

                          <button
                            className="btn btn-primary"
                            disabled={diagLoading}
                            onClick={handleChangeIP}
                          >
                            ⚡ Изменить IP ({selectedIP} ➔ {newIP || '...'})
                          </button>
                        </div>
                      </div>
                    </>
                  )}

                  {/* INSPECTOR TAB 2: CAMERA PASSPORT */}
                  {inspectorTab === 'passport' && (
                    <div className="section-box" style={{ flex: 1 }}>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                        }}
                      >
                        <span className="section-title">Паспорт ONVIF / ISAPI</span>
                        <button
                          className="btn btn-sm"
                          disabled={diagLoading}
                          onClick={() => handleInspectPassport(selectedIP)}
                        >
                          🔄 Обновить
                        </button>
                      </div>
                      <pre
                        className="mono"
                        style={{
                          fontSize: '11.5px',
                          lineHeight: 1.55,
                          whiteSpace: 'pre-wrap',
                          color: 'var(--text-primary)',
                          background: 'var(--bg-elevated)',
                          padding: '10px',
                          borderRadius: '8px',
                          border: '1px solid var(--border)',
                          flex: 1,
                          overflowY: 'auto',
                        }}
                      >
                        {passportText || 'Нажмите «Обновить» для беспарольного опроса камеры.'}
                      </pre>
                    </div>
                  )}

                  {/* INSPECTOR TAB 3: CABLE DIAGNOSTICS */}
                  {inspectorTab === 'cable' && (
                    <div className="section-box" style={{ flex: 1 }}>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                        }}
                      >
                        <span className="section-title">Диагностика кабеля RJ-45 / MTU 1500</span>
                        <button
                          className="btn btn-sm"
                          disabled={diagLoading}
                          onClick={() => handleCableTest(selectedIP)}
                        >
                          🔄 Запустить тест
                        </button>
                      </div>

                      {cableStats && (
                        <div
                          style={{
                            padding: '10px',
                            borderRadius: '8px',
                            border: '1px solid var(--border)',
                            background:
                              cableStats.status === 'ok'
                                ? 'var(--emerald-soft)'
                                : cableStats.status === 'warning'
                                ? 'var(--amber-soft)'
                                : 'var(--red-soft)',
                            color:
                              cableStats.status === 'ok'
                                ? 'var(--emerald)'
                                : cableStats.status === 'warning'
                                ? 'var(--amber)'
                                : 'var(--red)',
                            fontSize: '12px',
                            fontWeight: 700,
                            lineHeight: 1.45,
                          }}
                        >
                          <div>Потери 64B: {cableStats.smallLoss}% · Потери 1472B (MTU): {cableStats.heavyLoss}%</div>
                          <div style={{ marginTop: '4px', fontWeight: 600 }}>{cableStats.verdict}</div>
                        </div>
                      )}

                      <pre
                        className="mono"
                        style={{
                          fontSize: '11px',
                          lineHeight: 1.5,
                          whiteSpace: 'pre-wrap',
                          color: 'var(--text-primary)',
                          background: 'var(--bg-elevated)',
                          padding: '10px',
                          borderRadius: '8px',
                          border: '1px solid var(--border)',
                          flex: 1,
                          overflowY: 'auto',
                        }}
                      >
                        {cableText || 'Нажмите «Запустить тест» для проверки обжима RJ-45 и линии PoE.'}
                      </pre>
                    </div>
                  )}
                </div>
              </aside>
            </>
          )}

          {/* TAB 2: INTERACTIVE CABLE PLUG CAPTURE */}
          {navTab === 'cable' && (
            <div className="main-pane">
              <div className="section-box" style={{ padding: '20px' }}>
                <div className="section-title">// ПОИСК КАМЕРЫ ПО ФИЗИЧЕСКОМУ КАБЕЛЮ («ОТКЛЮЧИ → ВКЛЮЧИ»)</div>
                <div style={{ fontSize: '16px', fontWeight: 800, marginTop: '4px' }}>
                  Мгновенное определение неизвестной камеры в пучке кабелей на коммутаторе
                </div>
                <div style={{ color: 'var(--text-secondary)', lineHeight: 1.6, maxWidth: '820px' }}>
                  Когда на объекте десятки камер без подписей или камера находится в неизвестной чужой подсети:
                  <br />
                  <b>Шаг 1:</b> Нажмите кнопку <b>«Начать перехват по кабелю»</b> ниже.
                  <br />
                  <b>Шаг 2:</b> Выдерните патч-корд искомой камеры из PoE-коммутатора (или ноутбука) на <b>3 секунды</b> и воткните обратно.
                  <br />
                  <b>Шаг 3:</b> Радар зафиксирует Gratuitous ARP / L2-пакет инициализации камеры, автоматически пропишет точечный мост <code>/32</code> и выделит её!
                </div>

                <div style={{ display: 'flex', alignItems: 'center', gap: '12px', marginTop: '8px' }}>
                  <button
                    className="btn btn-primary"
                    style={{ padding: '10px 20px', fontSize: '13.5px' }}
                    disabled={plugScanning}
                    onClick={handlePlugScan}
                  >
                    {plugScanning
                      ? `⏳ Ожидание включения кабеля (${plugCountdown} сек)...`
                      : '🔌 Начать перехват по кабелю (24 сек)'}
                  </button>
                  <button
                    className="btn"
                    onClick={() => {
                      setNavTab('radar');
                    }}
                  >
                    Перейти к таблице ({cameras.length})
                  </button>
                </div>
              </div>
            </div>
          )}

          {/* TAB 3: SUBNET 254 IP MATRIX & FREE IP POOL */}
          {navTab === 'ipmap' && (
            <div className="main-pane">
              <div className="section-box">
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    flexWrap: 'wrap',
                    gap: '12px',
                  }}
                >
                  <div>
                    <div className="section-title">// КАРТА ПОДСЕТИ /24 И ПОДБОР СВОБОДНЫХ IP (1..254)</div>
                    <div style={{ fontSize: '14px', fontWeight: 800, marginTop: '2px' }}>
                      Выберите количество новых камер для автоматического подбора непрерывного блока IP:
                    </div>
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                    {[1, 4, 8, 16, 32, 64].map((n) => (
                      <button
                        key={n}
                        className={`btn btn-sm mono ${freeCount === n ? 'btn-primary' : ''}`}
                        onClick={() => handleFindFreeIPs(n)}
                      >
                        {n} IP
                      </button>
                    ))}
                    <input
                      type="number"
                      className="input mono"
                      style={{ width: '74px', fontWeight: 800 }}
                      min={1}
                      max={245}
                      value={freeCount}
                      onChange={(e) => setFreeCount(Math.max(1, parseInt(e.target.value || '1', 10)))}
                    />
                    <input
                      type="text"
                      className="input mono"
                      style={{ width: '145px' }}
                      placeholder="Подсеть (авто)"
                      value={freeSubnet}
                      onChange={(e) => setFreeSubnet(e.target.value)}
                    />
                    <button
                      className="btn btn-primary"
                      disabled={freeLoading}
                      onClick={() => handleFindFreeIPs()}
                    >
                      {freeLoading ? '⏳ Проверка 254 IP...' : '🎯 Подобрать свободные IP'}
                    </button>
                  </div>
                </div>

                {freePool && (
                  <div
                    style={{
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      flexWrap: 'wrap',
                      gap: '10px',
                      paddingTop: '10px',
                      borderTop: '1px solid var(--border-subtle)',
                    }}
                  >
                    <div style={{ fontSize: '12.5px' }}>
                      Подсеть: <b className="mono">{freePool.subnet}</b> (Шлюз:{' '}
                      <span className="mono">{freePool.gateway}</span>) · Свободно:{' '}
                      <b style={{ color: 'var(--emerald)' }}>{freePool.total_free_in_subnet}</b> из 254 ·
                      Рекомендованный диапазон ({freePool.recommended_list?.length || 0} шт.):{' '}
                      <b className="mono" style={{ color: 'var(--emerald)' }}>
                        {freePool.range_start} ➔ {freePool.range_end}
                      </b>
                    </div>
                    <button
                      className="btn btn-sm"
                      onClick={() =>
                        copyText(
                          (freePool.recommended_list || []).join(', '),
                          'Скопирован пул свободных IP'
                        )
                      }
                    >
                      📋 Скопировать список IP
                    </button>
                  </div>
                )}
              </div>

              {/* Legend + 254 Grid */}
              <div className="section-box" style={{ flex: 1, overflowY: 'auto' }}>
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                    flexWrap: 'wrap',
                    gap: '10px',
                    marginBottom: '6px',
                  }}
                >
                  <span className="section-title">Интерактивная матрица всех 254 адресов подсети</span>
                  <div style={{ display: 'flex', gap: '12px', fontSize: '11.5px' }}>
                    <span>🟢 Свободен</span>
                    <span style={{ color: 'var(--emerald)', fontWeight: 800 }}>
                      🟩 Рекомендованный пул
                    </span>
                    <span style={{ color: 'var(--blue)', fontWeight: 700 }}>🟦 Камера / NVR</span>
                    <span style={{ color: 'var(--red)' }}>🟥 Занят другим узлом</span>
                    <span style={{ color: 'var(--amber)' }}>🟨 Шлюз / ПК</span>
                  </div>
                </div>

                <div className="ip-matrix">
                  {subnetMatrix.map((cell) => (
                    <div
                      key={cell.host}
                      className={`ip-cell mono ${cell.status}`}
                      title={`${cell.ip} — ${cell.label || cell.status}`}
                      onClick={() => {
                        if (cell.status === 'free' || cell.status === 'recommended') {
                          setNewIP(cell.ip);
                          copyText(cell.ip, 'Выбран свободный IP');
                        } else if (cell.status === 'camera' || cell.status === 'nvr') {
                          setSelectedIP(cell.ip);
                          if (cell.mac) setSelectedMAC(cell.mac);
                          setNavTab('radar');
                        }
                      }}
                    >
                      .{cell.host}
                    </div>
                  ))}
                </div>
              </div>
            </div>
          )}

          {/* TAB 4: BATCH ACTIVATION & CONFLICT RESOLVER */}
          {navTab === 'batch' && (
            <div className="main-pane">
              <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '14px' }}>
                <div className="section-box">
                  <div className="section-title">// 01 · ПАКЕТНАЯ АКТИВАЦИЯ КАМЕР «ИЗ КОРОБКИ»</div>
                  <div style={{ fontSize: '13.5px', fontWeight: 800 }}>
                    Активация новых неактивированных камер (192.168.1.64 / 192.168.1.108)
                  </div>
                  <div style={{ color: 'var(--text-secondary)', fontSize: '12px', lineHeight: 1.5 }}>
                    Поочередно изолирует каждую новую камеру по физическому MAC-адресу, задает пароль администратора через ISAPI и при необходимости присваивает адреса по порядку (+1).
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginTop: '6px' }}>
                    <div>
                      <div className="info-label">Новый пароль для камер (от 8 символов)</div>
                      <input
                        type="text"
                        className="input"
                        style={{ width: '100%' }}
                        placeholder="Например: ProCam2026!"
                        value={batchPass}
                        onChange={(e) => setBatchPass(e.target.value)}
                      />
                    </div>
                    <div>
                      <div className="info-label">Начальный свободный IP (опционально)</div>
                      <input
                        type="text"
                        className="input mono"
                        style={{ width: '100%' }}
                        placeholder="192.168.1.101"
                        value={batchStartIP}
                        onChange={(e) => setBatchStartIP(e.target.value)}
                      />
                    </div>
                    <button className="btn btn-primary" onClick={handleBatchActivate}>
                      ⚡ Активировать партию камер
                    </button>
                  </div>
                </div>

                <div className="section-box">
                  <div className="section-title">// 02 · АВТОМАТИЧЕСКОЕ РАЗВЕДЕНИЕ КОНФЛИКТА IP</div>
                  <div style={{ fontSize: '13.5px', fontWeight: 800 }}>
                    Разведение нескольких камер с одного IP без отключения кабелей
                  </div>
                  <div style={{ color: 'var(--text-secondary)', fontSize: '12px', lineHeight: 1.5 }}>
                    Если несколько камер включены одновременно на одном заводском IP (например, <code>192.168.1.64</code>), программа привяжет каждую по MAC в ARP-таблице и разнесет на <code>Стартовый IP, +1, +2...</code>
                  </div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px', marginTop: '6px' }}>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px' }}>
                      <div>
                        <div className="info-label">Конфликтный IP</div>
                        <input
                          type="text"
                          className="input mono"
                          style={{ width: '100%' }}
                          value={conflictTargetIP}
                          onChange={(e) => setConflictTargetIP(e.target.value)}
                        />
                      </div>
                      <div>
                        <div className="info-label">Стартовый свободный IP</div>
                        <input
                          type="text"
                          className="input mono"
                          style={{ width: '100%' }}
                          value={conflictStartIP}
                          onChange={(e) => setConflictStartIP(e.target.value)}
                        />
                      </div>
                    </div>
                    <div>
                      <div className="info-label">Пароль камер</div>
                      <input
                        type="password"
                        className="input"
                        style={{ width: '100%' }}
                        placeholder="Пароль администратора камер"
                        value={conflictPass}
                        onChange={(e) => setConflictPass(e.target.value)}
                      />
                    </div>
                    <button className="btn btn-primary" onClick={handleBatchResolve}>
                      🔀 Развести конфликт по MAC-адресам
                    </button>
                  </div>
                </div>
              </div>

              <div className="section-box" style={{ flex: 1 }}>
                <div className="section-title">Журнал пакетной операции</div>
                <pre
                  className="mono"
                  style={{
                    flex: 1,
                    background: 'var(--bg-elevated)',
                    padding: '12px',
                    borderRadius: '8px',
                    border: '1px solid var(--border)',
                    whiteSpace: 'pre-wrap',
                    overflowY: 'auto',
                  }}
                >
                  {batchOutput || 'Выберите операцию выше для запуска пакетной настройки.'}
                </pre>
              </div>
            </div>
          )}

          {/* TAB 5: ENGINEERING CALCULATOR */}
          {navTab === 'calc' && (
            <div className="main-pane">
              <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: '12px' }}>
                {/* Card 1: HDD & Bitrate */}
                <div className="section-box">
                  <div className="section-title">1. Архив HDD и Поток</div>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px' }}>
                    <div>
                      <div className="info-label">Камер (шт.)</div>
                      <input
                        type="number"
                        className="input mono"
                        style={{ width: '100%', fontWeight: 800 }}
                        min={1}
                        max={512}
                        value={calcCams}
                        onChange={(e) => setCalcCams(Math.max(1, parseInt(e.target.value || '1', 10)))}
                      />
                    </div>
                    <div>
                      <div className="info-label">Дней архива</div>
                      <input
                        type="number"
                        className="input mono"
                        style={{ width: '100%', fontWeight: 800 }}
                        min={1}
                        max={365}
                        value={calcDays}
                        onChange={(e) => setCalcDays(Math.max(1, parseInt(e.target.value || '1', 10)))}
                      />
                    </div>
                    <div>
                      <div className="info-label">Разрешение</div>
                      <select
                        className="select"
                        style={{ width: '100%' }}
                        value={calcRes}
                        onChange={(e) => setCalcRes(parseFloat(e.target.value))}
                      >
                        <option value={2}>2 МП (1080p)</option>
                        <option value={4}>4 МП (2K QHD)</option>
                        <option value={5}>5 МП</option>
                        <option value={6}>6 МП</option>
                        <option value={8}>8 МП (4K UHD)</option>
                      </select>
                    </div>
                    <div>
                      <div className="info-label">Кодек сжатия</div>
                      <select
                        className="select"
                        style={{ width: '100%' }}
                        value={calcCodec}
                        onChange={(e) => setCalcCodec(e.target.value as any)}
                      >
                        <option value="h265plus">H.265+ (Smart)</option>
                        <option value="h265">H.265</option>
                        <option value="h264">H.264</option>
                      </select>
                    </div>
                  </div>
                  <button
                    className="btn btn-sm"
                    onClick={() => {
                      if (counts.cam > 0) setCalcCams(counts.cam);
                    }}
                  >
                    Взять число камер из скана ({counts.cam})
                  </button>
                </div>

                {/* Card 2: PoE & Cable Voltage Drop */}
                <div className="section-box">
                  <div className="section-title">2. Бюджет PoE и Кабель UTP</div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                    <div>
                      <div className="info-label">Тип камер и подсветки</div>
                      <select
                        className="select"
                        style={{ width: '100%' }}
                        value={calcWatts}
                        onChange={(e) => setCalcWatts(parseFloat(e.target.value))}
                      >
                        <option value={6.5}>ИК-подсветка 802.3af (до 6.5 Вт)</option>
                        <option value={10.5}>ColorVu / Dual-Light LED (до 10.5 Вт)</option>
                        <option value={22.0}>Поворотные PTZ PoE+ (до 22 Вт)</option>
                      </select>
                    </div>
                    <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px' }}>
                      <div>
                        <div className="info-label">Длина трассы (м)</div>
                        <input
                          type="number"
                          className="input mono"
                          style={{ width: '100%' }}
                          min={5}
                          max={300}
                          value={calcCableLen}
                          onChange={(e) =>
                            setCalcCableLen(Math.max(1, parseInt(e.target.value || '10', 10)))
                          }
                        />
                      </div>
                      <div>
                        <div className="info-label">Материал жилы</div>
                        <select
                          className="select"
                          style={{ width: '100%' }}
                          value={calcCableMat}
                          onChange={(e) => setCalcCableMat(e.target.value as any)}
                        >
                          <option value="cu">Медь 100% Cu</option>
                          <option value="cca">Алюминий CCA</option>
                        </select>
                      </div>
                    </div>
                  </div>
                </div>

                {/* Card 3: Lens & DORI */}
                <div className="section-box">
                  <div className="section-title">3. Объектив и DORI</div>
                  <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                    <div>
                      <div className="info-label">Фокусное расстояние (мм)</div>
                      <select
                        className="select"
                        style={{ width: '100%' }}
                        value={calcFocal}
                        onChange={(e) => setCalcFocal(parseFloat(e.target.value))}
                      >
                        <option value={2.8}>2.8 мм — Широкий угол ~103°</option>
                        <option value={4.0}>4.0 мм — Средний угол ~83°</option>
                        <option value={6.0}>6.0 мм — Направленный ~54°</option>
                        <option value={12.0}>12.0 мм — Дальнобойный ~25°</option>
                      </select>
                    </div>
                    <div>
                      <div className="info-label">Дистанция до объекта (м)</div>
                      <input
                        type="number"
                        className="input mono"
                        style={{ width: '100%' }}
                        min={1}
                        max={150}
                        value={calcDist}
                        onChange={(e) => setCalcDist(Math.max(1, parseFloat(e.target.value || '10')))}
                      />
                    </div>
                  </div>
                </div>
              </div>

              {/* Summary Dashboard */}
              <div className="section-box">
                <div className="section-title">// ИТОГОВЫЙ ИНЖЕНЕРНЫЙ РАСЧЕТ СИСТЕМЫ</div>
                <div
                  style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(4, 1fr)',
                    gap: '10px',
                    marginTop: '4px',
                  }}
                >
                  <div className="info-cell">
                    <div className="info-label">Чистый архив ({calcDays} дн.)</div>
                    <div
                      className="mono"
                      style={{ fontSize: '18px', fontWeight: 900, color: 'var(--emerald)' }}
                    >
                      {calcResults.rawTotalTB.toFixed(2)} ТБ
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
                      {Math.round(calcResults.totalGBPerDay)} ГБ / сутки
                    </div>
                  </div>

                  <div className="info-cell">
                    <div className="info-label">Рекомендуемый HDD</div>
                    <div style={{ fontSize: '14px', fontWeight: 800, marginTop: '2px' }}>
                      {calcResults.recDisk}
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
                      С учетом реальной емкости
                    </div>
                  </div>

                  <div className="info-cell">
                    <div className="info-label">Сеть и Регистратор NVR</div>
                    <div className="mono" style={{ fontSize: '16px', fontWeight: 900 }}>
                      {calcResults.totalMbps} Мбит/с
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--emerald)' }}>
                      {calcResults.nvrChannels}-кан. NVR (от {calcResults.recNvrBandwidth} Мбит/с)
                    </div>
                  </div>

                  <div className="info-cell">
                    <div className="info-label">Бюджет PoE и Напряжение</div>
                    <div
                      className="mono"
                      style={{ fontSize: '16px', fontWeight: 900, color: 'var(--amber)' }}
                    >
                      {calcResults.totalPoEWatts} Вт (БП от {calcResults.recSwitchWatts} Вт)
                    </div>
                    <div style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
                      На камере ({calcCableLen}м): ~{calcResults.vAtCam} В (-{calcResults.vDrop} В)
                    </div>
                  </div>
                </div>

                <div
                  style={{
                    background: 'var(--bg-elevated)',
                    border: '1px solid var(--border)',
                    borderRadius: '8px',
                    padding: '12px',
                    lineHeight: 1.6,
                    color: 'var(--text-secondary)',
                  }}
                >
                  <div>
                    • <b>Объектив {calcFocal} мм (~{calcResults.angleDeg}°) на {calcDist} м:</b> ширина
                    захвата сцены <b>{calcResults.sceneWidthM} м</b> (плотность{' '}
                    <b>{calcResults.ppm} пикс/м</b>).
                  </div>
                  <div>
                    • <b>Дистанции DORI:</b> Идентификация лица/номера: до <b>{calcResults.doriId} м</b> ·
                    Распознавание человека: до <b>{calcResults.doriRec} м</b> · Обнаружение: до{' '}
                    <b>{calcResults.doriDet} м</b>.
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* TAB 6: PDF REPORT & HELP */}
          {navTab === 'help' && (
            <div className="main-pane">
              <div className="section-box">
                <div className="section-title">// ЭКСПОРТ ИНЖЕНЕРНОЙ ДОКУМЕНТАЦИИ</div>
                <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
                  <button className="btn btn-primary" onClick={handleDownloadPDF}>
                    📄 Сформировать и открыть PDF-паспорт объекта
                  </button>
                  <button className="btn" onClick={handleExportCSV}>
                    📊 Выгрузить таблицу камер в CSV (Excel)
                  </button>
                  <button className="btn" onClick={() => window.open('/download/manual', '_blank')}>
                    📘 Открыть руководство инженера (PDF)
                  </button>
                </div>
              </div>

              <div className="section-box">
                <div className="section-title">// КЛЮЧЕВЫЕ ОСОБЕННОСТИ PROCAM RADAR v5.0</div>
                <div style={{ display: 'flex', flexDirection: 'column', gap: '10px', lineHeight: 1.55 }}>
                  <div>
                    <b>1. Строгая дедупликация устройств (IP + MAC):</b> ответы ONVIF WS-Discovery,
                    Hikvision SADP, Dahua DHIP и системная таблица ARP автоматически объединяются в единую
                    карточку камеры без дублирования строк.
                  </div>
                  <div>
                    <b>2. Одновременная работа Кабеля Ethernet и Wi-Fi (/32 маршрутизация):</b> при
                    подключении к камере в чужой подсети создается точечный маршрут <code>/32</code> через
                    проводной интерфейс, сохраняя интернет по Wi-Fi.
                  </div>
                  <div>
                    <b>3. Открытие по MAC при конфликте IP:</b> позволяет зайти в веб-интерфейс конкретной
                    камеры, даже если несколько новых камер сидят на одном заводском адресе{' '}
                    <code>192.168.1.64</code> или <code>192.168.1.108</code>.
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* TAB 7: AUTHOR DOSSIER & ECOSYSTEM ("ОБО МНЕ И ПРОЕКТЫ") */}
          {navTab === 'about' && (
            <div className="main-pane">
              {/* Author Hero Card */}
              <div
                className="section-box"
                style={{
                  padding: '20px',
                  borderColor: 'rgba(16, 185, 129, 0.35)',
                  background:
                    'linear-gradient(135deg, rgba(16,185,129,0.08) 0%, var(--bg-surface) 60%)',
                }}
              >
                <div
                  style={{
                    display: 'flex',
                    alignItems: 'flex-start',
                    justifyContent: 'space-between',
                    flexWrap: 'wrap',
                    gap: '16px',
                  }}
                >
                  <div style={{ maxWidth: '720px' }}>
                    <div className="section-title">// АВТОР И АРХИТЕКТОР ЭКОСИСТЕМЫ · YEEE.KZ</div>
                    <div
                      style={{
                        fontSize: '24px',
                        fontWeight: 900,
                        letterSpacing: '-0.02em',
                        marginTop: '4px',
                      }}
                    >
                      Ернияз Талғатулы
                    </div>
                    <div
                      style={{
                        fontSize: '13.5px',
                        fontWeight: 700,
                        color: 'var(--emerald)',
                        marginTop: '2px',
                      }}
                    >
                      Инженер систем безопасности & Fullstack-разработчик · Алматы, Казахстан
                    </div>
                    <div
                      style={{
                        color: 'var(--text-secondary)',
                        lineHeight: 1.6,
                        marginTop: '10px',
                        fontSize: '13px',
                      }}
                    >
                      Разработка профессиональных программно-аппаратных комплексов видеонаблюдения (
                      <b>ProCam CCTV</b>, <b>ProCam Radar</b>, <b>ProCam Viewer</b>), сетевых утилит L2/L3-уровня,
                      IoT-устройств и высоконагруженных веб-платформ. Полный цикл: от низкоуровневых сетевых
                      протоколов (<code>ARP</code>, <code>ONVIF</code>, <code>SADP</code>, <code>DHIP</code>,{' '}
                      <code>ISAPI</code>, <code>RTSP</code>) до современных кроссплатформенных интерфейсов.
                    </div>
                  </div>

                  {/* Direct Contact Box */}
                  <div
                    style={{
                      background: 'var(--bg-elevated)',
                      border: '1px solid var(--border)',
                      borderRadius: '11px',
                      padding: '14px 16px',
                      minWidth: '300px',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '8px',
                    }}
                  >
                    <div className="section-title">Прямая связь с автором</div>
                    <div style={{ fontSize: '12.5px' }}>
                      🌐 Портал:{' '}
                      <a
                        href="#yeee"
                        className="mono"
                        style={{ color: 'var(--emerald)', fontWeight: 800, textDecoration: 'none' }}
                        onClick={(e) => {
                          e.preventDefault();
                          openExternalURL('https://yeee.kz');
                        }}
                      >
                        https://yeee.kz ↗
                      </a>
                    </div>
                    <div style={{ fontSize: '12.5px' }}>
                      ✉️ Почта:{' '}
                      <span
                        className="mono"
                        style={{ cursor: 'pointer', fontWeight: 700 }}
                        onClick={() => copyText('yeeerniyaz@gmail.com', 'Скопирован Email')}
                      >
                        yeeerniyaz@gmail.com
                      </span>
                    </div>
                    <div style={{ fontSize: '12.5px' }}>
                      📞 Телефон (Алматы):{' '}
                      <span
                        className="mono"
                        style={{ fontWeight: 800, cursor: 'pointer' }}
                        onClick={() => copyText('+77066066323', 'Скопирован телефон')}
                      >
                        +7 (706) 606-63-23
                      </span>{' '}
                      /{' '}
                      <span
                        className="mono"
                        style={{ fontWeight: 800, cursor: 'pointer' }}
                        onClick={() => copyText('+77766066323', 'Скопирован телефон')}
                      >
                        +7 (776) 606-63-23
                      </span>
                    </div>
                    <div style={{ display: 'flex', gap: '6px', marginTop: '4px', flexWrap: 'wrap' }}>
                      <button
                        className="btn btn-sm btn-primary"
                        onClick={() => openExternalURL('https://yeee.kz')}
                      >
                        🌐 yeee.kz ↗
                      </button>
                      <button
                        className="btn btn-sm"
                        onClick={() => openExternalURL('https://wa.me/77066066323')}
                      >
                        💬 WhatsApp ↗
                      </button>
                      <button
                        className="btn btn-sm"
                        onClick={() => openExternalURL('https://t.me/yeeeerniyaz')}
                      >
                        ✈️ Telegram @yeeeerniyaz ↗
                      </button>
                      <button
                        className="btn btn-sm"
                        onClick={() => openExternalURL('https://github.com/Yeeerniyaz')}
                      >
                        💻 GitHub ↗
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              {/* Projects Grid */}
              <div>
                <div className="section-title" style={{ marginBottom: '10px' }}>
                  // ЭКОСИСТЕМА ПРОЕКТОВ И РАЗРАБОТОК АВТОРА
                </div>
                <div className="project-grid">
                  <div className="project-card">
                    <div>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          marginBottom: '6px',
                        }}
                      >
                        <span style={{ fontSize: '15px', fontWeight: 900 }}>ProCam CCTV</span>
                        <span className="badge badge-local">ВИДЕОНАБЛЮДЕНИЕ & СКУД</span>
                      </div>
                      <div style={{ color: 'var(--text-secondary)', fontSize: '12px', lineHeight: 1.55 }}>
                        Проектирование, монтаж и пусконаладка систем IP-видеонаблюдения, СКУД, слаботочных сетей
                        и серверных под ключ в Алматы. Интерактивный 3D-конфигуратор и онлайн-расчет сметы.
                      </div>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span className="mono" style={{ fontSize: '11px', color: 'var(--emerald)' }}>
                        procam.yeee.kz
                      </span>
                      <button
                        className="btn btn-sm btn-primary"
                        onClick={() => openExternalURL('https://procam.yeee.kz')}
                      >
                        Открыть сайт ↗
                      </button>
                    </div>
                  </div>

                  <div className="project-card">
                    <div>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          marginBottom: '6px',
                        }}
                      >
                        <span style={{ fontSize: '15px', fontWeight: 900 }}>ProCam Radar v5.0</span>
                        <span className="badge badge-camera">GO + WEB / DESKTOP</span>
                      </div>
                      <div style={{ color: 'var(--text-secondary)', fontSize: '12px', lineHeight: 1.55 }}>
                        Кроссплатформенный инженерный комплекс (Windows .exe & Linux) для мгновенного поиска
                        IP-камер и NVR без дубликатов, работы через 2×LAN + 2×Wi-Fi + Tailscale, диагностики
                        кабеля RJ-45 и генерации PDF-паспортов.
                      </div>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span className="mono" style={{ fontSize: '11px', color: 'var(--emerald)' }}>
                        github.com/Yeeerniyaz
                      </span>
                      <button
                        className="btn btn-sm"
                        onClick={() => openExternalURL('https://github.com/Yeeerniyaz/procam-radar')}
                      >
                        Репозиторий ↗
                      </button>
                    </div>
                  </div>

                  <div className="project-card">
                    <div>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          marginBottom: '6px',
                        }}
                      >
                        <span style={{ fontSize: '15px', fontWeight: 900 }}>ProCam Viewer</span>
                        <span className="badge badge-nvr">RTSP & ВИДЕОСТЕНА</span>
                      </div>
                      <div style={{ color: 'var(--text-secondary)', fontSize: '12px', lineHeight: 1.55 }}>
                        Кроссплатформенная система видеомониторинга реального времени для просмотра потоков
                        IP-камер Hikvision, Dahua, HiWatch, Uniview и ONVIF с аппаратным ускорением.
                      </div>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span className="mono" style={{ fontSize: '11px', color: 'var(--emerald)' }}>
                        procam.yeee.kz/#procam-radar
                      </span>
                      <button
                        className="btn btn-sm"
                        onClick={() => openExternalURL('https://procam.yeee.kz/#procam-radar')}
                      >
                        Подробнее ↗
                      </button>
                    </div>
                  </div>

                  <div className="project-card">
                    <div>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          marginBottom: '6px',
                        }}
                      >
                        <span style={{ fontSize: '15px', fontWeight: 900 }}>Royal Banners</span>
                        <span className="badge badge-foreign">B2B & КОНСТРУКТОР</span>
                      </div>
                      <div style={{ color: 'var(--text-secondary)', fontSize: '12px', lineHeight: 1.55 }}>
                        Платформа аренды и производства премиальных пресс-воллов, бренд-воллов и выставочных
                        конструкций в Алматы с интерактивным 3D-превью и мгновенным расчетом стоимости.
                      </div>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span className="mono" style={{ fontSize: '11px', color: 'var(--emerald)' }}>
                        royalbanners.yeee.kz
                      </span>
                      <button
                        className="btn btn-sm"
                        onClick={() => openExternalURL('https://royalbanners.yeee.kz/')}
                      >
                        Открыть сайт ↗
                      </button>
                    </div>
                  </div>

                  <div className="project-card">
                    <div>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          marginBottom: '6px',
                        }}
                      >
                        <span style={{ fontSize: '15px', fontWeight: 900 }}>TezTap Platform</span>
                        <span className="badge badge-camera">HIGHLOAD SAAS</span>
                      </div>
                      <div style={{ color: 'var(--text-secondary)', fontSize: '12px', lineHeight: 1.55 }}>
                        Сервисная платформа автоматизации заказов, диспетчеризации и управления городскими
                        услугами с микросервисной архитектурой и мгновенным откликом.
                      </div>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span className="mono" style={{ fontSize: '11px', color: 'var(--emerald)' }}>
                        yeee.kz
                      </span>
                      <button
                        className="btn btn-sm"
                        onClick={() => openExternalURL('https://yeee.kz')}
                      >
                        Портал автора ↗
                      </button>
                    </div>
                  </div>

                  <div className="project-card">
                    <div>
                      <div
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          justifyContent: 'space-between',
                          marginBottom: '6px',
                        }}
                      >
                        <span style={{ fontSize: '15px', fontWeight: 900 }}>
                          Justice & Smart Mirror VECTOR
                        </span>
                        <span className="badge badge-nvr">LEGALTECH & IOT</span>
                      </div>
                      <div style={{ color: 'var(--text-secondary)', fontSize: '12px', lineHeight: 1.55 }}>
                        Цифровая юридическая экосистема Justice Company и аппаратно-программный комплекс «умного
                        зеркала» VECTOR с компьютерным зрением, датчиками и голосовым ассистентом.
                      </div>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span className="mono" style={{ fontSize: '11px', color: 'var(--emerald)' }}>
                        yeee.kz
                      </span>
                      <button
                        className="btn btn-sm"
                        onClick={() => openExternalURL('https://yeee.kz')}
                      >
                        Все кейсы ↗
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>

        {/* LIVE RTSP VIDEO & STREAM PLAYER MODAL */}
        {rtspModalOpen && (
          <div className="modal-backdrop" onClick={() => setRtspModalOpen(false)}>
            <div className="modal-card" onClick={(e) => e.stopPropagation()}>
              <div className="modal-header">
                <div>
                  <div className="section-title">// ЖИВОЕ ВИДЕО И RTSP-ПЛЕЕР КАМЕРЫ</div>
                  <div style={{ fontSize: '16px', fontWeight: 900, marginTop: '2px' }}>
                    Камера <span className="mono">{selectedIP}</span>{' '}
                    {selectedCameraObj ? `· ${selectedCameraObj.vendor} (${selectedCameraObj.model})` : ''}
                  </div>
                </div>
                <div style={{ display: 'flex', gap: '8px' }}>
                  <button
                    className="btn btn-primary"
                    onClick={() => handleLaunchExternalRTSP(selectedIP)}
                  >
                    🖥️ Открыть во внешнем плеере (FFplay / MPV / VLC)
                  </button>
                  <button className="btn" onClick={() => setRtspModalOpen(false)}>
                    ✕ Закрыть
                  </button>
                </div>
              </div>

              <div className="modal-body">
                {/* Step 1: Camera Password & Stream Settings right inside the modal */}
                <div className="cred-bar">
                  <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                    <span style={{ fontWeight: 800, color: 'var(--emerald)' }}>
                      🔑 Пароль камеры для RTSP:
                    </span>
                    <input
                      type="text"
                      className="input mono"
                      style={{ width: '90px', padding: '5px 8px' }}
                      placeholder="Логин"
                      value={camUser}
                      onChange={(e) => setCamUser(e.target.value)}
                    />
                    <input
                      type={showPass ? 'text' : 'password'}
                      className="input mono"
                      style={{
                        width: '210px',
                        padding: '5px 9px',
                        borderColor: 'var(--emerald)',
                        fontWeight: 700,
                      }}
                      placeholder="Введите пароль от камеры..."
                      value={camPass}
                      onChange={(e) => setCamPass(e.target.value)}
                    />
                    <button
                      type="button"
                      className="btn btn-sm"
                      onClick={() => setShowPass(!showPass)}
                    >
                      {showPass ? '🙈 Скрыть' : '👁️ Показать'}
                    </button>
                    {['12345', 'admin', '123456', 'admin12345'].map((pw) => (
                      <button
                        key={pw}
                        type="button"
                        className={`preset-chip mono ${camPass === pw ? 'active' : ''}`}
                        onClick={() => {
                          setCamPass(pw);
                          setMjpegError(false);
                          setMjpegTick(Date.now());
                          handleCheckRTSP(selectedIP, pw);
                        }}
                      >
                        {pw}
                      </button>
                    ))}
                  </div>

                  <div style={{ display: 'flex', alignItems: 'center', gap: '6px', flexWrap: 'wrap' }}>
                    <select
                      className="select"
                      style={{ padding: '5px 8px', fontSize: '12px' }}
                      value={rtspStreamSub}
                      onChange={(e) => {
                        const v = e.target.value as 'main' | 'sub';
                        setRtspStreamSub(v);
                        setMjpegError(false);
                        setMjpegTick(Date.now());
                        handleCheckRTSP(selectedIP, undefined, v, undefined);
                      }}
                    >
                      <option value="main">Поток 101 (Основной HD)</option>
                      <option value="sub">Поток 102 (Субпоток)</option>
                    </select>

                    <select
                      className="select"
                      style={{ padding: '5px 8px', fontSize: '12px' }}
                      value={rtspProfile}
                      onChange={(e) => {
                        const v = e.target.value as 'auto' | 'hikvision' | 'dahua' | 'onvif';
                        setRtspProfile(v);
                        setMjpegError(false);
                        setMjpegTick(Date.now());
                        handleCheckRTSP(selectedIP, undefined, undefined, v);
                      }}
                    >
                      <option value="auto">Профиль: Авто</option>
                      <option value="hikvision">Hikvision / HiWatch</option>
                      <option value="dahua">Dahua / IMOU</option>
                      <option value="onvif">ONVIF Generic</option>
                    </select>

                    <button
                      type="button"
                      className="btn btn-sm btn-primary"
                      disabled={rtspChecking}
                      onClick={() => {
                        setMjpegError(false);
                        setMjpegTick(Date.now());
                        handleCheckRTSP(selectedIP);
                      }}
                    >
                      🔄 Подключить поток
                    </button>
                  </div>
                </div>

                {/* RTSP Connection Status Banner */}
                {rtspStatus && (
                  <div
                    style={{
                      padding: '10px 14px',
                      borderRadius: '9px',
                      border: '1px solid var(--border)',
                      background: rtspStatus.ok ? 'var(--emerald-soft)' : 'var(--amber-soft)',
                      color: rtspStatus.ok ? 'var(--emerald)' : 'var(--amber)',
                      fontWeight: 700,
                      fontSize: '12.5px',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'space-between',
                      gap: '10px',
                      flexWrap: 'wrap',
                    }}
                  >
                    <span>
                      {rtspStatus.ok ? '✔ ' : '⚠️ '}
                      {rtspStatus.message}
                    </span>
                    {rtspStatus.rtsp_url && (
                      <button
                        className="btn btn-sm mono"
                        onClick={() => copyText(rtspStatus.rtsp_url, 'Скопирована RTSP-ссылка')}
                      >
                        📋 Скопировать RTSP URL
                      </button>
                    )}
                  </div>
                )}

                {/* Built-in Live Video Viewport */}
                <div
                  style={{
                    background: '#050608',
                    border: '1px solid var(--border)',
                    borderRadius: '11px',
                    minHeight: '410px',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    position: 'relative',
                    overflow: 'hidden',
                  }}
                >
                  {!mjpegError ? (
                    <img
                      key={`${selectedIP}-${camUser}-${camPass}-${rtspStreamSub}-${rtspProfile}-${mjpegTick}`}
                      src={`/api/stream.mjpeg?ip=${encodeURIComponent(
                        selectedIP
                      )}&user=${encodeURIComponent(camUser || 'admin')}&pass=${encodeURIComponent(
                        camPass.trim()
                      )}&sub=${encodeURIComponent(rtspStreamSub)}&profile=${encodeURIComponent(
                        rtspProfile
                      )}&iface=${encodeURIComponent(getTargetIface(selectedIP))}&t=${mjpegTick}`}
                      alt="RTSP Live Stream"
                      style={{
                        maxWidth: '100%',
                        maxHeight: '460px',
                        objectFit: 'contain',
                        display: 'block',
                      }}
                      onError={() => setMjpegError(true)}
                    />
                  ) : (
                    <div
                      style={{
                        padding: '32px',
                        textAlign: 'center',
                        maxWidth: '580px',
                        color: 'var(--text-secondary)',
                      }}
                    >
                      <div
                        style={{
                          fontSize: '16px',
                          fontWeight: 800,
                          color: 'var(--text-primary)',
                          marginBottom: '8px',
                        }}
                      >
                        Видеопоток требует пароль от камеры ({selectedIP})
                      </div>
                      <div style={{ lineHeight: 1.55, marginBottom: '16px' }}>
                        Введите пароль администратора камеры в поле <b>«🔑 Пароль камеры для RTSP»</b> выше и
                        нажмите <b>«🔄 Подключить поток»</b> или откройте поток во внешнем плеере{' '}
                        <b>FFplay / MPV / VLC</b>.
                      </div>
                      <div style={{ display: 'inline-flex', gap: '8px', flexWrap: 'wrap' }}>
                        <button
                          className="btn btn-primary"
                          onClick={() => {
                            setMjpegError(false);
                            setMjpegTick(Date.now());
                            handleCheckRTSP(selectedIP);
                          }}
                        >
                          🔄 Повторить подключение в окне
                        </button>
                        <button
                          className="btn"
                          onClick={() => handleLaunchExternalRTSP(selectedIP)}
                        >
                          🖥️ Открыть во внешнем плеере (FFplay / MPV)
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}

        {/* COLLAPSIBLE LOG DRAWER */}
        {showLog && (
          <div className="log-drawer mono">
            {logs.map((line, i) => (
              <div key={i}>{line}</div>
            ))}
          </div>
        )}

        {/* NATIVE DESKTOP STATUS FOOTER */}
        <footer className="status-footer">
          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', overflow: 'hidden' }}>
            <span className="dot online" />
            <span style={{ fontWeight: 700, color: 'var(--text-primary)' }}>
              ProCam Radar v5.0
            </span>
            <span>·</span>
            <span style={{ whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
              {statusMessage}
            </span>
          </div>

          <div style={{ display: 'flex', alignItems: 'center', gap: '10px', flexShrink: 0 }}>
            <span style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>
              Разработчик: <b style={{ color: 'var(--text-primary)' }}>Ернияз Талғатулы</b> · Алматы
            </span>
            <button
              className="btn btn-sm"
              onClick={() => setNavTab('about')}
              style={{
                padding: '2px 8px',
                borderColor: 'rgba(16, 185, 129, 0.4)',
                color: 'var(--emerald)',
              }}
            >
              👤 Все проекты & обо мне ↗
            </button>
            <span className="mono">
              Устройств: <b>{counts.all}</b> (Камер: {counts.cam}, NVR: {counts.nvr})
            </span>
            <button
              className="btn btn-sm"
              onClick={() => setShowLog(!showLog)}
              style={{ padding: '2px 8px' }}
            >
              {showLog ? '▼ Скрыть консоль' : '▲ Журнал событий'}
            </button>
          </div>
        </footer>
      </div>
    </div>
  );
}
