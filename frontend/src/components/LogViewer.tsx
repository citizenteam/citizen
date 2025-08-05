import { useEffect, useState } from 'preact/hooks';

interface LogViewerProps {
  logs: string;
  isLive?: boolean;
  title?: string;
  onClose?: () => void;
  className?: string;
  style?: any;
}

// Log parsing function for better formatting
const parseLogLine = (line: string) => {
  // Docker build log pattern: 2025-06-29T01:11:07.373416048Z [1G [1G-----> message
  const dockerBuildRegex = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s+(.*)$/;
  const dockerMatch = line.match(dockerBuildRegex);
  
  if (dockerMatch) {
    let [, timestamp, message] = dockerMatch;
    // ANSI escape sequences'i temizle ([1G, [2K, \x1b[0m gibi)
    message = message.replace(/\[\d*[A-Za-z]/g, '').replace(/\x1b\[[0-9;]*m/g, '').trim();
    
    // Special categories for Dockerfile build steps
    let logType = 'build';
    let isImportant = false;
    
    if (message.includes('---->') || message.includes('=====>')) {
      logType = 'build-step';
      isImportant = true;
    } else if (message.includes('/docker-entrypoint.sh')) {
      logType = 'entrypoint';
    } else if (message.includes('nginx') || message.includes('Configuration complete')) {
      logType = 'nginx';
    } else if (message.includes('GET ') || message.includes('POST ')) {
      logType = 'access';
    }
    
    return {
      timestamp,
      process: logType,
      message: message || '',
      isStructured: true,
      isDockerLog: true,
      isImportant
    };
  }
  
      // Standard Citizen log pattern: 2025-06-27T19:15:07.384395679Z app[web.1]: message
    const citizenLogRegex = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s+(app\[[^\]]+\]):\s*(.*)$/;
    const citizenMatch = line.match(citizenLogRegex);
    
    if (citizenMatch) {
      const [, timestamp, process, message] = citizenMatch;
    return {
      timestamp,
      process,
      message: message || '',
      isStructured: true,
      isDockerLog: false,
      isImportant: false
    };
  }
  
  return {
    timestamp: '',
    process: '',
    message: line,
    isStructured: false,
    isDockerLog: false,
    isImportant: false
  };
};

// Format timestamp for display
const formatTimestamp = (timestamp: string) => {
  if (!timestamp) return '';
  
  try {
    const date = new Date(timestamp);
    const today = new Date();
    const isToday = date.toDateString() === today.toDateString();
    
    if (isToday) {
      return date.toLocaleTimeString('tr-TR', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
      });
    } else {
      return date.toLocaleDateString('tr-TR', {
        day: '2-digit',
        month: '2-digit'
      }) + ' ' + date.toLocaleTimeString('tr-TR', {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
      });
    }
  } catch {
    return timestamp;
  }
};

export default function LogViewer({ logs, isLive = false, title, onClose, className = "", style = {} }: LogViewerProps) {
  const [autoScroll, setAutoScroll] = useState(true);

  useEffect(() => {
    if (autoScroll && logs) {
      const logContainer = document.getElementById('log-container');
      if (logContainer) {
        logContainer.scrollTop = logContainer.scrollHeight;
      }
    }
  }, [logs, autoScroll]);

  return (
    <div 
      className={`p-4 md:p-6 rounded-lg md:rounded-2xl ${className}`}
      style={{
        backgroundColor: 'rgba(15, 23, 42, 0.95)', 
        border: '1px solid rgba(255, 255, 255, 0.1)',
        ...style
      }}
    >
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 mb-3 md:mb-4">
        <div className="flex items-center gap-2 md:gap-3">
          <h4 className="text-sm md:text-base font-semibold text-white">
            {title || 'Logs'}
          </h4>
          {isLive && (
            <div className="flex items-center gap-1 md:gap-2">
              <div 
                className="w-1.5 h-1.5 md:w-2 md:h-2 rounded-full"
                style={{ 
                  backgroundColor: '#22c55e',
                  animation: 'pulse 2s infinite'
                }}
              ></div>
              <span className="text-xs text-green-400">Live</span>
            </div>
          )}
        </div>
        
        <div className="flex items-center gap-2 justify-end">
          <button
            onClick={() => setAutoScroll(!autoScroll)}
            className="px-2 md:px-3 py-1 rounded-md md:rounded-lg text-xs whitespace-nowrap"
            style={{
              backgroundColor: autoScroll ? 'rgba(34, 197, 94, 0.1)' : 'rgba(107, 114, 128, 0.1)',
              color: autoScroll ? '#22c55e' : '#6b7280',
              border: `1px solid ${autoScroll ? 'rgba(34, 197, 94, 0.2)' : 'rgba(107, 114, 128, 0.2)'}`
            }}
          >
            Auto-scroll: {autoScroll ? 'ON' : 'OFF'}
          </button>
          
          {onClose && (
            <button
              onClick={onClose}
              className="text-gray-400 hover:text-gray-200 p-1"
              style={{ fontSize: '16px', lineHeight: '1' }}
            >
              ×
            </button>
          )}
        </div>
      </div>

      {/* Log Content */}
      <div 
        id="log-container"
        className="p-3 md:p-4 rounded-lg md:rounded-xl overflow-y-auto"
        style={{
          backgroundColor: '#0d1117',
          border: '1px solid rgba(255, 255, 255, 0.1)',
          fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
          fontSize: '11px',
          lineHeight: '1.5',
          minHeight: '250px',
          maxHeight: '400px'
        }}
      >
        {logs ? (
          <div>
            {logs.split('\n').map((line, index) => {
              if (!line.trim()) return <div key={index} style={{ height: '4px' }}></div>;
              
              const parsed = parseLogLine(line);
              
              if (parsed.isStructured) {
                return (
                  <div key={index} className="mb-0.5 md:mb-1">
                    {/* Mobile: Vertical Layout */}
                    <div className="block sm:hidden">
                      <div className="flex items-center gap-2 mb-1">
                        <span style={{ 
                          color: '#a855f7', 
                          fontWeight: '500',
                          fontSize: '9px',
                          opacity: '0.8'
                        }}>
                          {formatTimestamp(parsed.timestamp)}
                        </span>
                        <span style={{ 
                          color: parsed.isDockerLog ? (
                            parsed.process === 'build-step' ? '#10b981' : 
                            parsed.process === 'entrypoint' ? '#8b5cf6' : 
                            parsed.process === 'nginx' ? '#06b6d4' : 
                            parsed.process === 'access' ? '#6b7280' : 
                            '#f59e0b'
                          ) : '#06b6d4', 
                          fontWeight: '500',
                          fontSize: '9px',
                          opacity: '0.8'
                        }}>
                          {parsed.isDockerLog ? `[${parsed.process}]` : parsed.process}
                        </span>
                      </div>
                      <div style={{ 
                        color: parsed.isDockerLog ? (
                          parsed.process === 'build-step' ? '#f3f4f6' : 
                          parsed.process === 'access' ? '#9ca3af' : 
                          '#e5e7eb'
                        ) : '#e5e7eb',
                        wordBreak: 'break-all',
                        fontWeight: parsed.isImportant ? '600' : 'normal',
                        fontSize: parsed.isImportant ? '11px' : '10px',
                        paddingLeft: '4px'
                      }}>
                        {parsed.message}
                      </div>
                    </div>

                    {/* Desktop: Horizontal Layout */}
                    <div className="hidden sm:flex items-start">
                      <span style={{ 
                        color: '#a855f7', 
                        fontWeight: '500',
                        minWidth: '70px',
                        marginRight: '6px',
                        opacity: '0.9',
                        fontSize: '11px'
                      }}>
                        {formatTimestamp(parsed.timestamp)}
                      </span>
                      
                      <span style={{ 
                        color: parsed.isDockerLog ? (
                          parsed.process === 'build-step' ? '#10b981' : 
                          parsed.process === 'entrypoint' ? '#8b5cf6' : 
                          parsed.process === 'nginx' ? '#06b6d4' : 
                          parsed.process === 'access' ? '#6b7280' : 
                          '#f59e0b'
                        ) : '#06b6d4', 
                        fontWeight: '500',
                        minWidth: '60px',
                        marginRight: '6px',
                        opacity: '0.9',
                        fontSize: '11px'
                      }}>
                        {parsed.isDockerLog ? `[${parsed.process}]` : parsed.process}
                      </span>
                      
                      <span style={{ 
                        color: parsed.isDockerLog ? (
                          parsed.process === 'build-step' ? '#f3f4f6' : 
                          parsed.process === 'access' ? '#9ca3af' : 
                          '#e5e7eb'
                        ) : '#e5e7eb',
                        flex: '1',
                        wordBreak: 'break-all',
                        fontWeight: parsed.isImportant ? '600' : 'normal',
                        fontSize: parsed.isImportant ? '12px' : '11px'
                      }}>
                        {parsed.message}
                      </span>
                    </div>
                  </div>
                );
              } else {
                // Non-structured lines (deploy messages, etc.)
                const isDeployMessage = line.includes('===') || line.includes('---') || line.includes('Deploy') || line.includes('Repository:') || line.includes('Branch:') || line.includes('Builder:') || line.includes('Buildpack:');
                
                return (
                  <div key={index} className="mb-0.5 md:mb-1" style={{ 
                    color: isDeployMessage ? '#fbbf24' : '#9ca3af', 
                    fontWeight: isDeployMessage ? '500' : 'normal',
                    fontStyle: isDeployMessage ? 'normal' : 'italic',
                    fontSize: isDeployMessage ? '11px' : '10px',
                    wordBreak: 'break-word'
                  }}>
                    {line}
                  </div>
                );
              }
            })}
          </div>
        ) : (
          <div className="text-center text-gray-500 italic p-4 md:p-5" style={{ fontSize: '11px' }}>
            {isLive ? 'Waiting for logs...' : 'No logs available'}
          </div>
        )}
      </div>
      
      {/* Media Query Styles */}
      <style>{`
        @media (min-width: 768px) {
          #log-container {
            font-size: 12px !important;
            line-height: 1.5 !important;
            min-height: 300px !important;
            max-height: 500px !important;
          }
        }
      `}</style>
    </div>
  );
} 