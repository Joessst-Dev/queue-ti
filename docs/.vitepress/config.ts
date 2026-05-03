import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'

export default withMermaid(defineConfig({
  title: 'queue-ti',
  description: 'Self-hosted distributed message queue backed by PostgreSQL',
  base: '/queue-ti/',
  themeConfig: {
    nav: [
      { text: 'Home', link: '/' },
      { text: 'Guide', link: '/guide/getting-started' },
      { text: 'API', link: '/api/rest' },
      { text: 'Clients', link: '/clients/go' },
      { text: 'Development', link: '/development/contributing' },
    ],
    sidebar: {
      '/guide/': [
        {
          text: 'Guide',
          items: [
            { text: 'Getting Started', link: '/guide/getting-started' },
            { text: 'Concepts', link: '/guide/concepts' },
            { text: 'Consumer Groups', link: '/guide/consumer-groups' },
            { text: 'Authentication', link: '/guide/authentication' },
            { text: 'Schema Validation', link: '/guide/schema-validation' },
            { text: 'Observability', link: '/guide/observability' },
            { text: 'Deployment', link: '/guide/deployment' },
            { text: 'Configuration', link: '/guide/configuration' },
          ],
        },
      ],
      '/api/': [
        {
          text: 'API Reference',
          items: [
            { text: 'REST API', link: '/api/rest' },
            { text: 'gRPC Service', link: '/api/grpc' },
          ],
        },
      ],
      '/clients/': [
        {
          text: 'Client Libraries',
          items: [
            { text: 'Go Client', link: '/clients/go' },
            { text: 'Node.js Client', link: '/clients/nodejs' },
            { text: 'Python Client', link: '/clients/python' },
          ],
        },
      ],
      '/development/': [
        {
          text: 'Development',
          items: [
            { text: 'Contributing', link: '/development/contributing' },
            { text: 'Release Management', link: '/development/release' },
          ],
        },
      ],
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/Joessst-Dev/queue-ti' },
    ],
    search: {
      provider: 'local',
    },
  },
}))
