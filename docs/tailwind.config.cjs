/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./src/**/*.{astro,html,js,jsx,md,mdx,svelte,ts,tsx,vue}'],
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#FBF4F2',
          100: '#F8EDE3',
          200: '#DFD3C3',
          300: '#D0B8A8',
          400: '#CA8E7F',
          500: '#C5705D',
          600: '#B3574A',
          700: '#974838',
          800: '#7B3A2D',
          900: '#632F25',
        },
        secondary: {
          50: '#FCF9F6',
          100: '#F8EDE3',
          200: '#EFE1D1',
          300: '#DFD3C3',
          400: '#D0B8A8',
          500: '#C1A799',
          600: '#B29688',
          700: '#A08575',
          800: '#8D7466',
          900: '#7A6457',
        },
      },
      typography: {
        DEFAULT: {
          css: {
            maxWidth: 'none',
            color: '#3d2a20',
            a: {
              color: '#C5705D',
              textDecoration: 'none',
              transition: 'color 0.2s ease',
              '&:hover': {
                color: '#B3574A',
                textDecoration: 'underline',
              },
            },
            /* High-contrast headings: verified >10:1 contrast on #F8EDE3 cream bg */
            h1: { color: '#2c1810' },
            h2: { color: '#3b1f12' },
            h3: { color: '#4a2619' },
            h4: { color: '#4a2619' },
            h5: { color: '#4a2619' },
            h6: { color: '#4a2619' },
          },
        },
      },
    },
  },
  plugins: [require('@tailwindcss/typography')],
};
