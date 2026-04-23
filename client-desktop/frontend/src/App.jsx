import { useState, useEffect } from 'react';
import { Authenticate, GetMetrics, GetStatus } from '../wailsjs/go/main/App';
import { Cpu, Zap, HardDrive, Activity, Lock, ArrowRight, Signal } from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';
import './App.css';

function App() {
  const [code, setCode] = useState('');
  const [isLoggedIn, setIsLoggedIn] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [metrics, setMetrics] = useState(null);
  const [status, setStatus] = useState('');

  // Periodically fetch metrics if logged in
  useEffect(() => {
    let interval;
    if (isLoggedIn) {
      interval = setInterval(async () => {
        const m = await GetMetrics();
        if (m) setMetrics(m);
        const s = await GetStatus();
        setStatus(s);
      }, 1000);
    }
    return () => clearInterval(interval);
  }, [isLoggedIn]);

  const handleLogin = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError('');
    
    try {
      const result = await Authenticate(code);
      if (result.success) {
        setIsLoggedIn(true);
      } else {
        setError(result.message);
      }
    } catch (err) {
      setError('Erro fatal: ' + err);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div id="App">
      <div className="title-bar">
        <div className="logo-text">Auto Monitor</div>
        {isLoggedIn && (
          <div className="status-badge">
            <div className="pulse" />
            LIVE
          </div>
        )}
      </div>

      <div className="main-content">
        <AnimatePresence mode="wait">
          {!isLoggedIn ? (
            <motion.div 
              key="auth"
              initial={{ opacity: 0, y: 20 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -20 }}
              className="auth-container"
            >
              <div className="auth-card">
                <div style={{ display: 'flex', justifyContent: 'center', marginBottom: '1rem' }}>
                  <div style={{ background: 'rgba(59, 130, 246, 0.1)', padding: '1rem', borderRadius: '20px', color: 'var(--primary)' }}>
                    <Lock size={32} />
                  </div>
                </div>
                <h2 style={{ fontSize: '1.5rem', fontWeight: '800' }}>Ativar Agente</h2>
                <p style={{ color: 'var(--text-dim)', fontSize: '0.9rem', marginTop: '0.5rem' }}>
                  Insira o código da licença para iniciar o monitoramento desta máquina.
                </p>

                <form onSubmit={handleLogin}>
                  <div className="input-group">
                    <label style={{ fontSize: '0.75rem', fontWeight: '700', color: 'var(--text-dim)' }}>CÓDIGO APP</label>
                    <input 
                      type="text" 
                      placeholder="XXXX-XXXX-XXXX"
                      value={code}
                      onChange={(e) => setCode(e.target.value.toUpperCase())}
                      autoFocus
                    />
                  </div>
                  
                  {error && (
                    <div style={{ color: '#ef4444', fontSize: '0.8rem', marginTop: '1rem', background: 'rgba(239, 68, 68, 0.1)', padding: '8px', borderRadius: '8px' }}>
                      {error}
                    </div>
                  )}

                  <button className="btn-primary" type="submit" disabled={loading}>
                    {loading ? 'Validando...' : (
                      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: '8px' }}>
                        Conectar Agente <ArrowRight size={18} />
                      </div>
                    )}
                  </button>
                </form>
              </div>
            </motion.div>
          ) : (
            <motion.div 
              key="dashboard"
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              className="dashboard-container"
            >
              <div style={{ marginBottom: '1.5rem' }}>
                <h2 style={{ fontSize: '1.25rem', fontWeight: '800' }}>Monitoramento Ativo</h2>
                <p style={{ color: 'var(--text-dim)', fontSize: '0.8rem' }}>Enviando métricas para o servidor central.</p>
              </div>

              <div className="metrics-grid">
                <MetricCard 
                  title="CPU" 
                  value={metrics?.cpu_usage || 0} 
                  icon={<Cpu size={18} />} 
                  color="var(--primary)"
                />
                <MetricCard 
                  title="RAM" 
                  value={metrics?.ram_usage || 0} 
                  icon={<Zap size={18} />} 
                  color="var(--success)"
                />
                <MetricCard 
                  title="DISCO" 
                  value={metrics?.disk_usage || 0} 
                  icon={<HardDrive size={18} />} 
                  color="var(--warning)"
                />
                <div className="metric-card">
                  <div className="metric-header">
                    <Signal size={18} /> <span>REDE</span>
                  </div>
                  <div style={{ marginTop: '0.5rem' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.9rem', marginBottom: '4px' }}>
                      <span style={{ color: 'var(--text-dim)' }}>Upload:</span>
                      <span style={{ fontWeight: '800' }}>{((metrics?.network_tx || 0) / 1024).toFixed(1)} KB/s</span>
                    </div>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: '0.9rem' }}>
                      <span style={{ color: 'var(--text-dim)' }}>Download:</span>
                      <span style={{ fontWeight: '800' }}>{((metrics?.network_rx || 0) / 1024).toFixed(1)} KB/s</span>
                    </div>
                  </div>
                </div>
              </div>

              <div style={{ marginTop: '2rem', padding: '1rem', background: 'rgba(255,255,255,0.03)', borderRadius: '12px', fontSize: '0.75rem', color: 'var(--text-dim)', display: 'flex', alignItems: 'center', gap: '8px' }}>
                <Activity size={14} /> {status}
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    </div>
  );
}

function MetricCard({ title, value, icon, color }) {
  return (
    <div className="metric-card">
      <div className="metric-header">
        {icon} <span>{title}</span>
      </div>
      <div className="metric-value">{value.toFixed(1)}%</div>
      <div className="progress-track">
        <div 
          className="progress-fill" 
          style={{ width: `${value}%`, background: color }} 
        />
      </div>
    </div>
  );
}

export default App;
