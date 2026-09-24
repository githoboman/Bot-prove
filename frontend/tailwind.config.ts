import type { Config } from 'tailwindcss'

export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        cp: {
          black: '#080808',
          card: '#111111',
          border: '#1a1a1a',
          red: '#E53935',
          'red-bright': '#FF1744',
          'red-dark': '#B71C1C',
          'red-glow': 'rgba(229,57,53,0.15)',
          gray: '#9CA3AF',
          'gray-dark': '#6B7280',
        }
      },
      fontFamily: {
        sans: ['Inter', 'system-ui', '-apple-system', 'sans-serif'],
        mono: ['JetBrains Mono', 'Fira Code', 'monospace'],
      },
      animation: {
        'glow-pulse': 'glow-pulse 3s ease-in-out infinite',
        'color-shift': 'color-shift 8s ease-in-out infinite',
        'float': 'float 6s ease-in-out infinite',
        'float-delay': 'float 6s ease-in-out 2s infinite',
        'float-slow': 'float 8s ease-in-out 1s infinite',
        'pulse-slow': 'pulse 4s ease-in-out infinite',
        'fade-in-up': 'fade-in-up 0.6s ease-out',
        'smoke': 'smoke 12s ease-in-out infinite',
      },
      keyframes: {
        'glow-pulse': {
          '0%, 100%': { filter: 'drop-shadow(0 0 20px rgba(229,57,53,0.4))' },
          '50%': { filter: 'drop-shadow(0 0 40px rgba(229,57,53,0.8))' },
        },
        'color-shift': {
          '0%, 100%': { filter: 'hue-rotate(0deg) drop-shadow(0 0 25px rgba(229,57,53,0.5))' },
          '33%': { filter: 'hue-rotate(200deg) drop-shadow(0 0 35px rgba(53,100,229,0.5))' },
          '66%': { filter: 'hue-rotate(280deg) drop-shadow(0 0 35px rgba(153,53,229,0.5))' },
        },
        'float': {
          '0%, 100%': { transform: 'translateY(0px)' },
          '50%': { transform: 'translateY(-12px)' },
        },
        'fade-in-up': {
          from: { opacity: '0', transform: 'translateY(24px)' },
          to: { opacity: '1', transform: 'translateY(0)' },
        },
        'smoke': {
          '0%, 100%': { opacity: '0.3', transform: 'scale(1) translateY(0)' },
          '50%': { opacity: '0.6', transform: 'scale(1.1) translateY(-10px)' },
        },
      },
    },
  },
  plugins: [],
} satisfies Config
