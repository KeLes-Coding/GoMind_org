module.exports = {
  darkMode: 'class',
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        'bg-light': '#FCFCFC',
        'bg-dark': '#121212',
        'surface-light': '#FFFFFF',
        'surface-dark': '#1E1E1E',
        'text-primary-light': '#1A1A1A',
        'text-primary-dark': '#F5F5F5',
        'text-secondary-light': '#666666',
        'text-secondary-dark': '#A0A0A0',
        'border-light': '#E0E0E0',
        'border-dark': '#333333',
        'accent-light': '#FF6B00',
        'accent-dark': '#FF8C33',
      }
    },
  },
  plugins: [
    require('@tailwindcss/typography'),
  ],
}
