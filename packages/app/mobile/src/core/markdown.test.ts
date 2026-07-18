import { describe, expect, it } from 'vitest';

import { stripUnsupportedMarkdownEmoji } from './markdown';

describe('stripUnsupportedMarkdownEmoji', () => {
  it('removes emoji sequences without breaking heading syntax', () => {
    expect(stripUnsupportedMarkdownEmoji('## 🌧️ 上海天气\n### 📍 外滩')).toBe(
      '## 上海天气\n### 外滩',
    );
  });

  it('removes joined and flag emoji while preserving normal symbols', () => {
    expect(stripUnsupportedMarkdownEmoji('家庭 👨‍👩‍👧‍👦 国旗 🇨🇳 杭州→上海 + 高温')).toBe(
      '家庭  国旗  杭州→上海 + 高温',
    );
  });
});
