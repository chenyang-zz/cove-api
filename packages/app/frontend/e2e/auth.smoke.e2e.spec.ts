import { expect, test } from '@playwright/test'

const sessionStorageKey = 'cove.auth.session.v1'

function testIdentity() {
  const runId = (process.env.E2E_RUN_ID ?? 'local')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '')
    .slice(-20)
  const suffix = `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`
  const username = `e2e-${runId || 'local'}-${suffix}`.slice(0, 64)
  return {
    username,
    email: `${username}@example.test`,
    password: 'Cove-e2e-123!',
  }
}

async function expectAuthenticated(page: import('@playwright/test').Page, username: string) {
  await expect(page.getByRole('heading', { name: '你好，用户' })).toBeVisible()
  await expect(page.getByText(`@${username}`, { exact: true })).toBeVisible()
}

test.describe('authentication smoke', () => {
  test('registers, refreshes, restores, logs out, and logs back in @smoke', async ({ page }) => {
    const identity = testIdentity()

    await page.goto('/')
    await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()

    await page.getByRole('tab', { name: '注册' }).click()
    await expect(page.getByRole('heading', { name: '创建你的账号' })).toBeVisible()
    await page.getByLabel('用户名', { exact: true }).fill(identity.username)
    await page.getByLabel(/邮箱/).fill(identity.email)
    await page.getByLabel('密码', { exact: true }).fill(identity.password)
    await page.getByLabel('确认密码', { exact: true }).fill(identity.password)

    const registerResponsePromise = page.waitForResponse((response) =>
      response.request().method() === 'POST' && response.url().endsWith('/api/auth/register'),
    )
    await page.getByRole('button', { name: '创建账号' }).click()
    const registerResponse = await registerResponsePromise
    expect(registerResponse.status()).toBe(200)

    const registerEnvelope = await registerResponse.json() as {
      code: number
      data?: { user_id?: string; username?: string }
    }
    expect(registerEnvelope.code).toBe(0)
    expect(registerEnvelope.data?.user_id).toBeTruthy()
    expect(registerEnvelope.data?.username).toBe(identity.username)
    await expectAuthenticated(page, identity.username)

    const oldRefreshToken = await page.evaluate((key) => {
      const raw = window.localStorage.getItem(key)
      if (!raw) {
        throw new Error('authenticated session was not persisted')
      }
      const session = JSON.parse(raw) as { refreshToken: string; accessToken: string }
      session.accessToken = 'invalid-e2e-access-token'
      window.localStorage.setItem(key, JSON.stringify(session))
      return session.refreshToken
    }, sessionStorageKey)

    const refreshResponsePromise = page.waitForResponse((response) =>
      response.request().method() === 'POST' && response.url().endsWith('/api/auth/refresh'),
    )
    await page.reload()
    const refreshResponse = await refreshResponsePromise
    expect(refreshResponse.status()).toBe(200)
    await expectAuthenticated(page, identity.username)

    const refreshRotated = await page.evaluate(({ key, previousToken }) => {
      const raw = window.localStorage.getItem(key)
      if (!raw) {
        return false
      }
      const session = JSON.parse(raw) as { refreshToken?: string }
      return Boolean(session.refreshToken && session.refreshToken !== previousToken)
    }, { key: sessionStorageKey, previousToken: oldRefreshToken })
    expect(refreshRotated).toBe(true)

    await page.getByRole('button', { name: '退出登录' }).click()
    await expect(page.getByRole('heading', { name: '欢迎回来' })).toBeVisible()
    await expect.poll(() => page.evaluate((key) => window.localStorage.getItem(key), sessionStorageKey)).toBeNull()

    await page.getByLabel('用户名或邮箱').fill(identity.username)
    await page.getByLabel('密码', { exact: true }).fill(identity.password)
    const loginResponsePromise = page.waitForResponse((response) =>
      response.request().method() === 'POST' && response.url().endsWith('/api/auth/login'),
    )
    await page.getByRole('button', { name: '登录' }).click()
    const loginResponse = await loginResponsePromise
    expect(loginResponse.status()).toBe(200)
    await expectAuthenticated(page, identity.username)
  })
})
