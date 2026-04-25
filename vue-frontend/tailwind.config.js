module.exports = {
  darkMode: 'class',
  content: [
    "./index.html",
    "./src/**/*.{vue,js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        'primary': '#904D00',
        'primary-container': '#FF8C00',
        'on-primary': '#FFFFFF',
        'secondary': '#865224',
        'tertiary': '#00658F',
        'bg-light': '#F8F7F5',
        'bg-dark': '#0A0A0A',
        'surface-light': '#FFFFFF',
        'surface-dark': '#171717',
        'text-primary-light': '#191C1D',
        'text-primary-dark': '#FAFAFA',
        'text-secondary-light': '#74716D',
        'text-secondary-dark': '#A3A3A3',
        'border-light': '#E7E0D8',
        'border-dark': '#262626',
        'accent-light': '#FF8C00',
        'accent-dark': '#FF8C00',
      }
    },
  },
  plugins: [
    require('@tailwindcss/typography'),
  ],
}
