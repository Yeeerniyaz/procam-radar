export interface CameraEntry {
  ip: string;
  mac: string;
  vendor: string;
  model: string;
  serial: string;
  firmware: string;
  device_type: 'camera' | 'nvr';
  device_label: string;
  ports: number[];
  ping_ms?: number;
  activated?: string; // "active" | "inactive" | "unknown"
  zone: string;
  zone_type: 'local' | 'foreign' | 'conflict';
  iface: string;
  iface_kind: 'ethernet' | 'wired' | 'wifi' | 'vpn' | 'other';
  subnet_alias: string;
  web_link: string;
  rtsp_link: string;
  conflict: boolean;
}

export interface NetInterfaceInfo {
  name: string;
  kind: 'ethernet' | 'wired' | 'wifi' | 'vpn' | 'other';
  label: string;
  ip: string;
  all_ips?: string[];
  subnet: string;
  carrier: boolean;
}

export interface SysInfoResponse {
  app?: string;
  version: string;
  os: string;
  iface: string;
  ip: string;
  subnet: string;
  interfaces: NetInterfaceInfo[];
  active_bridges?: string[];
  cached_count?: number;
}

export interface HostCell {
  host: number;
  ip: string;
  status: 'gateway' | 'self' | 'camera' | 'nvr' | 'occupied' | 'recommended' | 'free';
  mac?: string;
  label?: string;
}

export interface FreePoolResponse {
  subnet: string;
  gateway: string;
  requested_count: number;
  total_free_in_subnet: number;
  occupied_in_subnet: number;
  range_start: string;
  range_end: string;
  recommended_list: string[];
  host_map?: HostCell[];
}

export interface CableTestResponse {
  ip: string;
  small_loss: number;
  small_avg_ms: number;
  heavy_loss: number;
  heavy_avg_ms: number;
  status: 'ok' | 'warning' | 'bad';
  verdict: string;
  raw_output: string;
}

export interface CamPassportResponse {
  ip: string;
  mac: string;
  vendor: string;
  model: string;
  serial: string;
  firmware: string;
  activated: string;
  http_server: string;
  rtsp_banner: string;
  tls_subject: string;
  onvif_time: string;
  ntp_drift: string;
  ports: number[];
  ping_ms: number;
  subnet_bridge: string;
  raw_output: string;
}
