import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/quickstart',
        'getting-started/first-project',
      ],
    },
    {
      type: 'category',
      label: 'API Reference',
      items: [
        'api/authentication',
        'api/projects',
        'api/tasks',
        'api/comments',
        'api/swim-lanes',
        'api/sprints',
        'api/tags',
        'api/wiki',
        'api/search',
        'api/team',
        'api/admin',
        'api/github-integration',
        'api/attachments',
        'api/notifications',
        'api/drawings',
        'api/settings',
        'api/error-handling',
      ],
    },
    {
      type: 'category',
      label: 'MCP Integration',
      items: [
        'mcp/overview',
        'mcp/tools-reference',
      ],
    },
    {
      type: 'category',
      label: 'Guides',
      items: [
        'guides/github-sync',
        'guides/wiki-collaboration',
        'guides/oauth-setup',
        'guides/api-keys',
      ],
    },
    'troubleshooting',
  ],
};

export default sidebars;
