import type { SidebarsConfig } from '@docusaurus/plugin-content-docs';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

/**
 * Mirrors gopay's interface-segregation model (see AGENT.md): the base
 * Provider surface first, then each optional capability interface, then the
 * providers that implement them, then testing.
 */
const sidebars: SidebarsConfig = {
  docsSidebar: [
    'intro',
    'getting-started',
    {
      type: 'category',
      label: 'Core',
      collapsed: false,
      items: ['client', 'payments', 'refunds', 'errors'],
    },
    {
      type: 'category',
      label: 'Capabilities',
      collapsed: false,
      items: [
        'customers',
        'payment-methods',
        'setup-intents',
        'subscriptions',
        'invoices',
        'listing-pagination',
        'webhooks',
      ],
    },
    {
      type: 'category',
      label: 'Providers',
      collapsed: false,
      items: ['stripe', 'paypal', 'razorpay'],
    },
    {
      type: 'category',
      label: 'Testing',
      collapsed: false,
      items: ['mock-provider'],
    },
  ],
};

export default sidebars;
