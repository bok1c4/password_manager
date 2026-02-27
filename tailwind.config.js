/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: '#0066CC',
          dark: '#4DA8FF',
        },
        surface: {
          light: '#F5F5F7',
          dark: '#16213E',
        },
        background: {
          light: '#FFFFFF',
          dark: '#1A1A2E',
        },
      },
    },
  },
  plugins: [],
}
