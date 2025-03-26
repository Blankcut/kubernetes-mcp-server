# Kubernetes Claude MCP Documentation

This directory contains the documentation site for the Kubernetes Claude MCP server. The site is built using [Astro](https://astro.build/), a modern static site generator.

## Development

### Prerequisites

- Node.js 16.x or later
- npm or yarn

### Getting Started

1. Install dependencies:

```bash
cd docs
npm install
```

2. Start the development server:

```bash
npm run dev
```

This will start a local server at http://localhost:4321 where you can preview the documentation site.

### Directory Structure

- `src/pages/` - Main pages for the site
- `src/content/docs/` - Markdown/MDX documentation files
- `src/components/` - Reusable UI components
- `src/layouts/` - Page layouts
- `src/styles/` - Global CSS and styles
- `public/` - Static assets like images

### Adding Documentation

To add a new documentation page:

1. Create a new Markdown or MDX file in `src/content/docs/`
2. Add frontmatter with title, description, and other metadata:

```markdown
---
title: Your Page Title
description: A brief description of the page
date: 2025-03-01
order: 10 # Controls the order in sidebar navigation
tags: ['tag1', 'tag2']
---

# Your Page Content

Write your documentation here...
```

3. Add the page to the sidebar in `src/components/Sidebar.astro`

### Building for Production

To build the site for production:

```bash
npm run build
```

This will generate static files in the `dist/` directory that can be deployed to any static hosting service.

### Deploying

The documentation site can be deployed to any static hosting service. Some options:

#### GitHub Pages

1. Build the site:

```bash
npm run build
```

2. Deploy to GitHub Pages:

```bash
npm install -D gh-pages
npx gh-pages -d dist
```

#### Netlify

1. Connect your repository to Netlify
2. Set the build command to `cd docs && npm run build`
3. Set the publish directory to `docs/dist`

#### Vercel

1. Connect your repository to Vercel
2. Set the build command to `cd docs && npm run build`
3. Set the output directory to `docs/dist`

## Customization

### Styling

The site uses Tailwind CSS for styling. You can customize the theme in `tailwind.config.cjs`.

### Components

Reusable components are located in `src/components/`. Feel free to create new components or modify existing ones.

### Layouts

Page layouts are in `src/layouts/`. The main documentation layout is `DocLayout.astro`.

## Contributing

Contributions to the documentation are welcome! Please follow these steps:

1. Fork the repository
2. Create a new branch for your changes
3. Make your changes
4. Submit a pull request

For content changes, please follow these guidelines:

- Use clear, concise language
- Follow the existing style and formatting
- Add appropriate frontmatter to new pages
- Update the sidebar if adding new pages
- Test your changes locally before submitting a PR

## License

This documentation is licensed under the MIT License. See the [LICENSE](../LICENSE) file for details.