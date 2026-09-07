// Only the backend's evidenced connection_method is eligible for a LAN label.
// ICE candidate type (including "host") is deliberately not an accepted method.
export function connectionMethodLabel(method: string): string {
  switch (method) {
    case 'lan_direct': return '局域网直连';
    case 'internet_p2p': return '互联网 P2P';
    case 'relay': return '尚未实现';
    default: return '直连 · 路径待确认';
  }
}
