'use client';

import { useState, useEffect, useCallback, useRef } from 'react';

interface SystemMetrics { cpu: number; ram: number; swap: number; }
interface Alert { id: string; timestamp: string; message: string; severity: 'info' | 'warning' | 'critical'; }
type ConnectionStatus = 'connecting' | 'connected' | 'disconnected';
type GlobalStatus = 'ok' | 'warning' | 'critical';

const getColor = (v: number) => v > 90 ? '#f87171' : v > 70 ? '#facc15' : '#34d399';
const getTextColor = (v: number) => v > 90 ? '#f87171' : v > 70 ? '#facc15' : '#34d399';
const computeStatus = (m: SystemMetrics): GlobalStatus =>
  m.cpu > 90 || m.ram > 90 ? 'critical' : m.cpu > 70 || m.ram > 70 ? 'warning' : 'ok';

function MetricCard({ label, value, unit }: { label: string; value: number; unit: string }) {
  return (
    <div style={{ background: '#1f2937', border: '1px solid #374151', borderRadius: 12, padding: 20 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 10 }}>
        <span style={{ color: '#9ca3af', fontFamily: 'monospace', fontSize: 11, textTransform: 'uppercase', letterSpacing: 2 }}>{label}</span>
        <span style={{ fontFamily: 'monospace', fontSize: 22, fontWeight: 700, color: getTextColor(value) }}>
          {value.toFixed(1)}<span style={{ fontSize: 13 }}>{unit}</span>
        </span>
      </div>
      <div style={{ width: '100%', height: 6, background: '#374151', borderRadius: 9999, overflow: 'hidden' }}>
        <div style={{ height: '100%', width: `${Math.min(value, 100)}%`, background: getColor(value), borderRadius: 9999, transition: 'width 0.5s ease', boxShadow: `0 0 8px ${getColor(value)}60` }} />
      </div>
    </div>
  );
}

export default function SentinelDashboard() {
  const [metrics, setMetrics] = useState<SystemMetrics>({ cpu: 0, ram: 0, swap: 0 });
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [conn, setConn] = useState<ConnectionStatus>('connecting');
  const esRef = useRef<EventSource | null>(null);
  const counterRef = useRef(0);

  const connect = useCallback(() => {
    esRef.current?.close();
    setConn('connecting');
    const API = process.env.NEXT_PUBLIC_API_URL || 'http://94.72.118.12:8080';
    const es = new EventSource(`${API}/api/events`);
    esRef.current = es;
    es.onopen = () => setConn('connected');
    es.onmessage = (e) => {
      try {
        const d = JSON.parse(e.data);
        if (d.type === 'system_metrics') setMetrics({ cpu: d.cpu ?? 0, ram: d.ram ?? 0, swap: d.swap ?? 0 });
        if (d.type === 'alert') {
          const a: Alert = { id: `${Date.now()}-${++counterRef.current}`, timestamp: d.timestamp, message: d.message, severity: d.severity || 'warning' };
          setAlerts(prev => [a, ...prev].slice(0, 50));
        }
      } catch { /* ignorar */ }
    };
    es.onerror = () => { setConn('disconnected'); es.close(); setTimeout(connect, 3000); };
  }, []);

  useEffect(() => { connect(); return () => esRef.current?.close(); }, [connect]);

  const status = conn === 'connected' ? computeStatus(metrics) : 'warning';
  const statusConfig = {
    ok:       { label: 'SISTEMA OK',     color: '#34d399' },
    warning:  { label: conn !== 'connected' ? (conn === 'connecting' ? 'CONECTANDO...' : 'DESCONECTADO') : 'ADVERTENCIA', color: '#facc15' },
    critical: { label: 'ALERTA CRÍTICA', color: '#f87171' },
  }[status];

  return (
    <main style={{ minHeight: '100vh', background: '#111827', color: '#f3f4f6', fontFamily: 'monospace', padding: 24 }}>
      <div style={{ maxWidth: 900, margin: '0 auto' }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 28 }}>
          <div>
            <h1 style={{ fontSize: 20, fontWeight: 700, letterSpacing: 2, margin: 0 }}>SentinelMX</h1>
            <p style={{ fontSize: 11, color: '#6b7280', margin: '4px 0 0', letterSpacing: 1 }}>MONITOREO EN TIEMPO REAL</p>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: 10, background: '#1f2937', border: `1px solid ${statusConfig.color}40`, borderRadius: 8, padding: '8px 16px' }}>
            <div style={{ width: 10, height: 10, borderRadius: '50%', background: statusConfig.color, animation: status === 'critical' ? 'pulse 1s infinite' : 'none' }} />
            <span style={{ color: statusConfig.color, fontSize: 12, letterSpacing: 2 }}>{statusConfig.label}</span>
          </div>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 16, marginBottom: 24 }}>
          <MetricCard label="CPU" value={metrics.cpu} unit="%" />
          <MetricCard label="RAM" value={metrics.ram} unit="%" />
          <MetricCard label="SWAP" value={metrics.swap} unit=" MB" />
        </div>

        <div style={{ background: '#1f2937', border: '1px solid #374151', borderRadius: 12, padding: 20 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
            <span style={{ fontSize: 11, color: '#9ca3af', textTransform: 'uppercase', letterSpacing: 2 }}>Feed de Alertas</span>
            <span style={{ fontSize: 11, color: '#6b7280' }}>{alerts.length} eventos</span>
          </div>
          {alerts.length === 0
            ? <p style={{ color: '#4b5563', textAlign: 'center', padding: '32px 0', fontSize: 13 }}>Sin alertas — sistema estable</p>
            : <div style={{ maxHeight: 320, overflowY: 'auto', display: 'flex', flexDirection: 'column', gap: 6 }}>
                {alerts.map(a => (
                  <div key={a.id} style={{ display: 'flex', gap: 12, padding: '8px 12px', borderRadius: 8, borderLeft: `3px solid ${a.severity === 'critical' ? '#f87171' : '#facc15'}`, background: a.severity === 'critical' ? '#450a0a20' : '#42200620' }}>
                    <span style={{ color: '#6b7280', fontSize: 11, flexShrink: 0 }}>{new Date(a.timestamp).toLocaleTimeString('es-ES')}</span>
                    <span style={{ fontSize: 12, color: a.severity === 'critical' ? '#fca5a5' : '#fde68a' }}>{a.message}</span>
                  </div>
                ))}
              </div>
          }
        </div>
        <style>{`@keyframes pulse{0%,100%{opacity:1}50%{opacity:.3}}`}</style>
      </div>
    </main>
  );
}
