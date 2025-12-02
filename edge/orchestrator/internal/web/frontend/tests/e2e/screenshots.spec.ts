import { expect, test } from '@playwright/test'

async function waitForEnabledCamera(page: Parameters<typeof test>[0]['page']) {
  const timeoutMs = 30_000
  const start = Date.now()

  // Poll backend directly so we are not dependent solely on UI timing.
  // This ensures at least one enabled camera exists before we try to use the UI.
  // eslint-disable-next-line no-constant-condition
  while (true) {
    const resp = await page.request.get('/api/cameras')
    if (resp.ok()) {
      const data = (await resp.json()) as {
        cameras?: Array<{ id: string; name: string; enabled: boolean }>
      }
      const enabled = (data.cameras || []).filter((c) => c.enabled)
      if (enabled.length > 0) {
        return enabled
      }
    }

    if (Date.now() - start > timeoutMs) {
      throw new Error('No enabled cameras returned by /api/cameras within 30s')
    }
    await page.waitForTimeout(1000)
  }
}

async function selectFirstCamera(page: Parameters<typeof test>[0]['page']) {
  // Ensure backend has at least one enabled camera
  const enabledCameras = await waitForEnabledCamera(page)

  // Verify we're on the correct page
  await page.waitForURL('**/screenshots', { timeout: 10_000 })

  // Wait for React root to be mounted (check for any React-rendered content)
  await page.waitForFunction(
    () => document.querySelector('#root')?.children.length > 0,
    { timeout: 10_000 }
  ).catch(() => {
    throw new Error('React app did not mount within 10s')
  })

  // Wait for the capture button to appear - this is the most reliable indicator
  // that the page has fully loaded and the capture section is rendered.
  const captureButton = page.getByTestId('capture-screenshot-button')
  await expect(captureButton).toBeVisible({ timeout: 30_000 })

  // Prefer the known-good USB camera ("5MP USB Camera") if available.
  // The shared Select component does not associate <label> with <select>,
  // so we use the first <select> on the page (camera selector).
  const cameraSelect = page.locator('select').first()
  await expect(cameraSelect).toBeVisible({ timeout: 15_000 })

  // Try selecting the 5MP USB camera by label; fall back to first enabled camera if not present.
  const options = await cameraSelect.locator('option').allInnerTexts()
  const usbIndex = options.findIndex((text) => /5mp usb camera/i.test(text))
  if (usbIndex >= 0) {
    await cameraSelect.selectOption({ index: usbIndex })
  } else {
    // Use the first enabled camera returned by the backend
    await cameraSelect.selectOption(enabledCameras[0].id)
  }
}

async function captureAndSaveNormalScreenshot(page: Parameters<typeof test>[0]['page']) {
  // Wait for capture button to be visible and enabled (page must be fully loaded)
  const captureButton = page.getByTestId('capture-screenshot-button')
  await expect(captureButton).toBeVisible({ timeout: 15_000 })
  await expect(captureButton).toBeEnabled({ timeout: 5_000 })
  await captureButton.click()

  // Wait for capture modal to appear and image preview to load
  const modal = page.getByRole('dialog', { name: /label screenshot/i })
  const errorBanner = page.getByTestId('screenshot-error-banner')

  // Wait until either modal appears or an error banner is shown
  await Promise.race([
    modal.waitFor({ state: 'visible', timeout: 15_000 }),
    errorBanner.waitFor({ state: 'visible', timeout: 15_000 }),
  ]).catch(() => {})

  if (await errorBanner.isVisible().catch(() => false)) {
    const message = await errorBanner.innerText()
    throw new Error(`Capture failed – error banner visible: ${message}`)
  }

  await expect(modal).toBeVisible({ timeout: 1_000 })

  const imagePreview = modal.getByRole('img', { name: /captured/i })
  await expect(imagePreview).toBeVisible()

  // Set label to "normal" (should be default, but make it explicit)
  // The Select component renders a plain <select> without aria label association,
  // so we select the first <select> inside the modal.
  const labelSelect = modal.locator('select').first()
  await expect(labelSelect).toBeVisible()
  await labelSelect.selectOption('normal')

  // Fill optional description
  const descriptionInput = modal.getByLabel(/description/i)
  if (await descriptionInput.isVisible()) {
    await descriptionInput.fill('Playwright E2E labeled screenshot')
  }

  // Save screenshot
  const saveButton = page.getByRole('button', { name: /^save screenshot$/i })
  await expect(saveButton).toBeEnabled()
  await saveButton.click()

  // Expect toast or success message to appear
  // (relies on Toast component using role=alert or visible success text)
  const successToast = page
    .getByRole('alert')
    .filter({ hasText: /screenshot saved/i })
    .first()
  await expect(successToast).toBeVisible({ timeout: 10_000 })
}

test.describe('Screenshots page - capture, dataset progress, sync, and error handling', () => {
  test.beforeEach(async ({ page }) => {
    // Capture console errors to help debug rendering issues
    page.on('console', (msg) => {
      if (msg.type() === 'error') {
        console.error(`[Browser Console Error]: ${msg.text()}`)
      }
    })
    page.on('pageerror', (error) => {
      console.error(`[Page Error]: ${error.message}`)
    })
  })

  test('captures and saves a labeled screenshot for a camera', async ({ page }) => {
    await page.goto('/screenshots', { waitUntil: 'networkidle' })

    await selectFirstCamera(page)
    await captureAndSaveNormalScreenshot(page)

    // Verify that at least one screenshot card exists in the list
    const screenshotCards = page.locator('[data-testid="screenshot-card"]')
    const cardCount = await screenshotCards.count()
    expect(cardCount).toBeGreaterThan(0)
  })

  test('dataset progress reaches ready state and enables sync button', async ({ page }) => {
    await page.goto('/screenshots', { waitUntil: 'networkidle' })

    await selectFirstCamera(page)

    // Capture enough "normal" screenshots to satisfy min_normal_snapshots
    // (in infra/local test config this should be set low, e.g. 3)
    // We loop a few times and break as soon as the UI reports "Ready for training".
    const maxCaptures = 5
    for (let i = 0; i < maxCaptures; i += 1) {
      await captureAndSaveNormalScreenshot(page)

      const readyText = page.getByText(/ready for training/i)
      if (await readyText.isVisible().catch(() => false)) {
        break
      }
    }

    // Expect dataset widget to report readiness
    const readyText = page.getByText(/ready for training/i)
    await expect(readyText).toBeVisible()

    // Sync button should now be enabled
    const syncButton = page.getByTestId('sync-dataset-status-button')
    await expect(syncButton).toBeVisible({ timeout: 5_000 })
    await expect(syncButton).toBeEnabled({ timeout: 5_000 })

    // Click sync and verify the Edge API call succeeds
    const [response] = await Promise.all([
      page.waitForResponse((resp) => {
        const url = resp.url()
        return (
          url.includes('/api/cameras/') &&
          url.endsWith('/dataset/sync') &&
          resp.request().method() === 'POST'
        )
      }),
      syncButton.click(),
    ])

    expect(response.status()).toBe(200)

    // Expect UI to show a success indicator about dataset sync
    const syncSuccessToast = page
      .getByRole('alert')
      .filter({ hasText: /dataset status synced/i })
      .first()
    await expect(syncSuccessToast).toBeVisible({ timeout: 10_000 })
  })

  test('shows error state when capture fails (HTTP 5xx)', async ({ page }) => {
    // Intercept snapshot capture and force 500
    await page.route('**/api/cameras/*/snapshot**', async (route) => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Internal snapshot error (simulated)' }),
      })
    })

    await page.goto('/screenshots', { waitUntil: 'networkidle' })
    await selectFirstCamera(page)

    const captureButton = page.getByTestId('capture-screenshot-button')
    await expect(captureButton).toBeVisible({ timeout: 15_000 })
    await expect(captureButton).toBeEnabled({ timeout: 5_000 })
    await captureButton.click()

    // User-facing error message at top of page
    const errorBanner = page.getByText(/failed to capture/i).first()
    await expect(errorBanner).toBeVisible()

    // Ensure no capture modal is opened (since snapshot failed)
    const modalTitle = page.getByRole('heading', { name: /label screenshot/i })
    await expect(modalTitle).toHaveCount(0)
  })

  test('shows validation / error toast when screenshot save fails', async ({ page }) => {
    // Let snapshot succeed, but fail the POST /screenshots
    await page.route('**/api/screenshots', async (route) => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Simulated save failure' }),
      })
    })

    await page.goto('/screenshots', { waitUntil: 'networkidle' })
    await selectFirstCamera(page)

    const captureButton = page.getByTestId('capture-screenshot-button')
    await expect(captureButton).toBeVisible({ timeout: 15_000 })
    await expect(captureButton).toBeEnabled({ timeout: 5_000 })
    await captureButton.click()

    const modal = page.getByRole('dialog', { name: /label screenshot/i })
    await expect(modal).toBeVisible()

    const imagePreview = page.getByRole('img', { name: /captured/i })
    await expect(imagePreview).toBeVisible()

    const saveButton = page.getByRole('button', { name: /^save screenshot$/i })
    await expect(saveButton).toBeEnabled()
    await saveButton.click()

    // Error should be shown either:
    // 1. As a toast with role=alert, or
    // 2. In the top error banner, or
    // 3. Inside the modal
    const errorToast = page.getByRole('alert').filter({ hasText: /failed to save screenshot|simulated save failure/i }).first()
    const errorBanner = page.getByTestId('screenshot-error-banner')
    const modalError = modal.getByText(/failed to save screenshot|simulated save failure/i).first()
    
    // Wait for any of these to appear
    await Promise.race([
      errorToast.waitFor({ state: 'visible', timeout: 10_000 }).catch(() => {}),
      errorBanner.waitFor({ state: 'visible', timeout: 10_000 }).catch(() => {}),
      modalError.waitFor({ state: 'visible', timeout: 10_000 }).catch(() => {}),
    ])
    
    // At least one error surface should be visible
    const hasError = 
      (await errorToast.isVisible().catch(() => false)) ||
      (await errorBanner.isVisible().catch(() => false)) ||
      (await modalError.isVisible().catch(() => false))
    
    expect(hasError).toBe(true)
  })

  test('sync button state matches dataset readiness', async ({ page }) => {
    await page.goto('/screenshots', { waitUntil: 'networkidle' })
    await selectFirstCamera(page)

    // Check dataset progress widget to determine expected button state
    const readyText = page.getByText(/ready for training/i)
    const inProgressText = page.getByText(/more normal snapshots needed/i)
    
    const isReady = await readyText.isVisible().catch(() => false)
    const isInProgress = await inProgressText.isVisible().catch(() => false)
    
    const syncButton = page.getByTestId('sync-dataset-status-button')
    
    if (await syncButton.count()) {
      await expect(syncButton).toBeVisible({ timeout: 5_000 })
      
      // Button should be enabled when ready, disabled when in progress
      if (isReady) {
        await expect(syncButton).toBeEnabled()
      } else if (isInProgress) {
        await expect(syncButton).toBeDisabled()
      }
      // If neither ready nor in-progress text is visible, dataset_status might be null
    }
  })
})

