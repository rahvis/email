import { describe, it, expect, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import useThemeStore from './theme'

describe('ThemeStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    // Reset document attribute
    document.documentElement.removeAttribute('theme-mode')
  })

  it('has correct initial state', () => {
    const store = useThemeStore()
    expect(store.theme).toBe('light')
    expect(store.themeOverrides).toBeDefined()
    expect(store.themeOverrides.common).toBeDefined()
  })

  describe('setTheme', () => {
    it('sets theme to dark', () => {
      const store = useThemeStore()
      store.setTheme('dark')
      expect(store.theme).toBe('dark')
    })

    it('sets theme to light', () => {
      const store = useThemeStore()
      store.setTheme('dark')
      store.setTheme('light')
      expect(store.theme).toBe('light')
    })
  })

  describe('themeOverrides structure', () => {
    it('contains expected common overrides', () => {
      const store = useThemeStore()
      const common = store.themeOverrides.common
      expect(common).toBeDefined()
      expect(common!.fontSize).toBe('15px')
      expect(common!.fontSizeSmall).toBe('13px')
      expect(common!.fontSizeMedium).toBe('15px')
      expect(common!.fontSizeLarge).toBe('16px')
      expect(common!.borderRadius).toBe('8px')
      expect(common!.baseColor).toBe('#ffffff')
      expect(common!.lineHeight).toBe('1.4')
    })

    it('contains Layout overrides', () => {
      const store = useThemeStore()
      expect(store.themeOverrides.Layout).toBeDefined()
    })

    it('contains Menu overrides', () => {
      const store = useThemeStore()
      const menu = store.themeOverrides.Menu
      expect(menu).toBeDefined()
      expect(menu!.fontSize).toBe('14px')
    })

    it('contains Card overrides', () => {
      const store = useThemeStore()
      const card = store.themeOverrides.Card
      expect(card).toBeDefined()
      expect(card!.borderColor).toBe('#e3e8ee')
      expect(card!.borderRadius).toBe('12px')
    })

    it('contains Form overrides', () => {
      const store = useThemeStore()
      const form = store.themeOverrides.Form
      expect(form).toBeDefined()
      expect(form!.labelFontWeight).toBe('400')
    })

    it('contains Radio overrides with transparent borders', () => {
      const store = useThemeStore()
      const radio = store.themeOverrides.Radio
      expect(radio).toBeDefined()
      expect(radio!.buttonBorderColor).toBe('transparent')
      expect(radio!.buttonBorderColorHover).toBe('transparent')
      expect(radio!.buttonBorderColorActive).toBe('transparent')
    })

    it('contains DataTable overrides', () => {
      const store = useThemeStore()
      const dt = store.themeOverrides.DataTable
      expect(dt).toBeDefined()
      expect(dt!.thPaddingMedium).toBe('12px 14px')
      expect(dt!.tdPaddingMedium).toBe('12px 14px')
      expect(dt!.borderRadius).toBe('12px')
    })

    it('contains Breadcrumb overrides', () => {
      const store = useThemeStore()
      expect(store.themeOverrides.Breadcrumb).toBeDefined()
      expect(store.themeOverrides.Breadcrumb!.fontSize).toBe('14px')
    })

    it('contains Progress overrides', () => {
      const store = useThemeStore()
      expect(store.themeOverrides.Progress).toBeDefined()
      expect(store.themeOverrides.Progress!.textColorLineInner).toBe('#fff')
    })
  })

  describe('changeTheme', () => {
    it('sets theme-mode attribute to dark when theme is dark', async () => {
      const store = useThemeStore()
      store.setTheme('dark')
      store.changeTheme()
      expect(document.documentElement.getAttribute('theme-mode')).toBe('dark')
    })

    it('sets theme-mode attribute to empty when theme is light', async () => {
      const store = useThemeStore()
      store.setTheme('light')
      store.changeTheme()
      expect(document.documentElement.getAttribute('theme-mode')).toBe('')
    })
  })
})
