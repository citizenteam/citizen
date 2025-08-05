import { useState, useRef, useEffect } from 'preact/hooks';

interface SelectOption {
  value: string;
  label: string;
}

interface CustomSelectProps {
  options: SelectOption[];
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  className?: string;
  disabled?: boolean;
}

export default function CustomSelect({ 
  options, 
  value, 
  onChange, 
  placeholder = "Select...",
  className = "",
  disabled = false 
}: CustomSelectProps) {
  const [isOpen, setIsOpen] = useState(false);
  const selectRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (selectRef.current && !selectRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const selectedOption = options.find(opt => opt.value === value);

  return (
    <div ref={selectRef} className={`relative ${className}`}>
      <button
        type="button"
        onClick={() => !disabled && setIsOpen(!isOpen)}
        disabled={disabled}
        className="w-full px-3 py-2 text-left rounded-lg border transition-all duration-200 flex items-center justify-between"
        style={{
          backgroundColor: disabled ? '#f3f4f6' : 'rgba(255, 255, 255, 0.8)',
          borderColor: isOpen ? '#3b82f6' : 'rgba(0, 0, 0, 0.08)',
          color: disabled ? '#9ca3af' : '#374151',
          fontSize: '14px',
          fontWeight: '500',
          cursor: disabled ? 'not-allowed' : 'pointer',
          opacity: disabled ? '0.6' : '1'
        }}
      >
        <span className="truncate">
          {selectedOption ? selectedOption.label : placeholder}
        </span>
        <svg 
          className={`w-4 h-4 transition-transform duration-200 flex-shrink-0 ml-2 ${isOpen ? 'rotate-180' : ''}`}
          fill="none" 
          stroke="currentColor" 
          viewBox="0 0 24 24"
        >
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {isOpen && (
        <div 
          className="absolute z-50 w-full mt-1 rounded-lg shadow-lg border overflow-hidden"
          style={{
            backgroundColor: '#ffffff',
            borderColor: 'rgba(0, 0, 0, 0.08)',
            maxHeight: '200px',
            overflowY: 'auto'
          }}
        >
          {options.map((option) => (
            <button
              key={option.value}
              type="button"
              onClick={() => {
                onChange(option.value);
                setIsOpen(false);
              }}
              className="w-full px-3 py-2 text-left hover:bg-gray-50 transition-colors duration-150"
              style={{
                backgroundColor: option.value === value ? 'rgba(59, 130, 246, 0.08)' : 'transparent',
                color: option.value === value ? '#3b82f6' : '#374151',
                fontSize: '14px',
                fontWeight: option.value === value ? '600' : '500',
                borderBottom: '1px solid rgba(0, 0, 0, 0.04)'
              }}
            >
              {option.label}
            </button>
          ))}
        </div>
      )}
    </div>
  );
} 