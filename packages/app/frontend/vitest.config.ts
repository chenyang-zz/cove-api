import { mergeConfig } from 'vite'
import { configDefaults, defineConfig } from 'vitest/config'

import viteConfig from './vite.config'

export default mergeConfig(
  viteConfig,
  defineConfig({
    test: {
      exclude: [...configDefaults.exclude, 'e2e/**'],
      execArgv: process.allowedNodeEnvironmentFlags.has('--no-experimental-webstorage')
        ? ['--no-experimental-webstorage']
        : [],
    },
  }),
)
