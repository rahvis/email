import { defineStore } from 'pinia'
import { GlobalThemeOverrides } from 'naive-ui'

const cssVarFallbacks: Record<string, string> = {
	'--font-brand': "'Inter Variable', sohne-var, 'SF Pro Display', system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
	'--color-primary-1': '#533afd',
	'--color-primary-hover-1': '#4434d4',
	'--color-primary-press-1': '#2e2b8c',
	'--color-primary-subdued-1': '#b9b9f9',
	'--color-brand-dark-1': '#1c1e54',
	'--color-success-1': '#12a66a',
	'--color-warning-1': '#9b6829',
	'--color-error-1': '#ea2261',
	'--color-info-1': '#665efd',
	'--color-bg-1': '#ffffff',
	'--color-bg-2': '#f6f9fc',
	'--color-text-1': '#0d253d',
	'--color-text-2': '#273951',
	'--color-text-3': '#64748d',
	'--color-border-1': '#e3e8ee',
	'--color-border-2': '#a8c3de',
	'--color-menu-1': 'rgba(255, 255, 255, 0.72)',
	'--color-menu-active-1': 'rgba(83, 58, 253, 0.22)',
	'--color-menu-active-2': '#ffffff',
	'--color-radio-1': '#eef3f8',
	'--color-radio-2': '#ffffff',
	'--color-button-text-1': '#ffffff',
	'--color-table-th-1': '#f6f9fc',
	'--color-table-td-1': '#ffffff',
	'--shadow-card': 'rgba(0, 55, 112, 0.08) 0 1px 3px',
}

export default defineStore(
	'ThemeStore',
	() => {
		const theme = ref<'light' | 'dark'>('light')

		const getCssVar = (name: string) => {
			return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || cssVarFallbacks[name] || ''
		}

		const getThemeOverrides = (): GlobalThemeOverrides => {
			return {
				common: {
					fontFamily: getCssVar('--font-brand'),
					lineHeight: '1.4',
					fontSize: '15px',
					fontSizeSmall: '13px',
					fontSizeMedium: '15px',
					fontSizeLarge: '16px',
					borderRadius: '8px',
					borderRadiusSmall: '6px',
					baseColor: getCssVar('--color-bg-1'),
					textColor1: getCssVar('--color-text-1'),
					textColor2: getCssVar('--color-text-2'),
					textColor3: getCssVar('--color-text-3'),
					primaryColor: getCssVar('--color-primary-1'),
					primaryColorHover: getCssVar('--color-primary-hover-1'),
					primaryColorPressed: getCssVar('--color-primary-press-1'),
					successColor: getCssVar('--color-success-1'),
					successColorHover: getCssVar('--color-success-1'),
					warningColor: getCssVar('--color-warning-1'),
					errorColor: getCssVar('--color-error-1'),
					infoColor: getCssVar('--color-info-1'),
				},
				Layout: {
					color: getCssVar('--color-bg-2'),
					textColor: getCssVar('--color-text-1'),
					headerColor: getCssVar('--color-bg-1'),
					siderColor: getCssVar('--color-brand-dark-1'),
					siderTextColor: getCssVar('--color-menu-1'),
				},
				Menu: {
					fontSize: '14px',
					borderRadius: '9999px',
					itemTextColor: getCssVar('--color-menu-1'),
					itemIconColor: getCssVar('--color-menu-1'),
					itemColorActive: getCssVar('--color-menu-active-1'),
					itemTextColorActive: getCssVar('--color-menu-active-2'),
					itemIconColorActive: getCssVar('--color-menu-active-2'),
					itemColorActiveHover: getCssVar('--color-menu-active-1'),
					itemTextColorActiveHover: getCssVar('--color-menu-active-2'),
					itemIconColorActiveHover: getCssVar('--color-menu-active-2'),
					itemColorActiveCollapsed: getCssVar('--color-menu-active-1'),
				},
				Card: {
					color: getCssVar('--color-bg-1'),
					borderColor: getCssVar('--color-border-1'),
					borderRadius: '12px',
					boxShadow: getCssVar('--shadow-card'),
				},
				Form: {
					feedbackHeightMedium: '20px',
					feedbackHeightLarge: '22px',
					feedbackFontSizeMedium: '13px',
					feedbackFontSizeLarge: '13px',
					feedbackPadding: '2px 0 0',
					labelFontSizeTopMedium: '13px',
					labelFontSizeLeftMedium: '13px',
					labelPaddingHorizontal: '0 20px 0 0',
					labelFontWeight: '400',
				},
				Input: {
					heightMedium: '40px',
					heightLarge: '44px',
					borderRadius: '6px',
					color: getCssVar('--color-bg-1'),
					colorFocus: getCssVar('--color-bg-1'),
					textColor: getCssVar('--color-text-1'),
					border: `1px solid ${getCssVar('--color-border-2')}`,
					borderHover: `1px solid ${getCssVar('--color-primary-1')}`,
					borderFocus: `1px solid ${getCssVar('--color-primary-1')}`,
					boxShadowFocus: '0 0 0 3px rgba(83, 58, 253, 0.12)',
				},
				Select: {
					peers: {
						InternalSelection: {
							heightMedium: '40px',
							borderRadius: '6px',
							border: `1px solid ${getCssVar('--color-border-2')}`,
							borderHover: `1px solid ${getCssVar('--color-primary-1')}`,
							borderActive: `1px solid ${getCssVar('--color-primary-1')}`,
							boxShadowActive: '0 0 0 3px rgba(83, 58, 253, 0.12)',
						},
					},
				},
				Radio: {
					buttonColor: getCssVar('--color-radio-1'),
					buttonColorActive: getCssVar('--color-radio-2'),
					buttonTextColor: getCssVar('--color-text-2'),
					buttonTextColorHover: getCssVar('--color-text-1'),
					buttonTextColorActive: getCssVar('--color-text-1'),
					buttonBorderColor: 'transparent',
					buttonBorderColorHover: 'transparent',
					buttonBorderColorActive: 'transparent',
					buttonBoxShadowHover: 'none',
					buttonBoxShadowFocus: 'none',
					buttonBorderRadius: '9999px',
					labelPadding: '0 0 0 8px',
				},
				Modal: {
					borderRadius: '12px',
				},
				Dialog: {
					contentMargin: '16px 0',
					textColor: getCssVar('--color-text-1'),
					borderRadius: '12px',
				},
				DataTable: {
					fontSizeMedium: '14px',
					thPaddingMedium: '12px 14px',
					tdPaddingMedium: '12px 14px',
					thColor: getCssVar('--color-table-th-1'),
					tdColor: getCssVar('--color-table-td-1'),
					tdColorHover: getCssVar('--color-table-th-1'),
					borderColor: getCssVar('--color-border-1'),
					borderRadius: '12px',
				},
				Breadcrumb: {
					fontSize: '14px',
				},
				Switch: {
					railColorActive: getCssVar('--color-primary-1'),
				},
				Tabs: {
					tabBorderRadius: '9999px',
					tabTextColorActiveLine: getCssVar('--color-primary-1'),
					barColor: getCssVar('--color-primary-1'),
				},
				Progress: {
					textColorLineInner: '#fff',
				},
				Button: {
					fontSizeMedium: '15px',
					fontSizeLarge: '16px',
					heightMedium: '40px',
					heightLarge: '44px',
					borderRadiusMedium: '9999px',
					borderRadiusLarge: '9999px',
					color: getCssVar('--color-bg-1'),
					colorHover: getCssVar('--color-bg-1'),
					colorPressed: getCssVar('--color-bg-1'),
					textColorPrimary: getCssVar('--color-button-text-1'),
					colorPrimary: getCssVar('--color-primary-1'),
					colorHoverPrimary: getCssVar('--color-primary-hover-1'),
					colorPressedPrimary: getCssVar('--color-primary-press-1'),
					borderPrimary: `1px solid ${getCssVar('--color-primary-1')}`,
					borderHoverPrimary: `1px solid ${getCssVar('--color-primary-hover-1')}`,
					borderPressedPrimary: `1px solid ${getCssVar('--color-primary-press-1')}`,
				},
				Pagination: {
					itemBorderRadius: '9999px',
					itemTextColorActive: getCssVar('--color-primary-1'),
				},
				Tag: {
					borderRadius: '9999px',
					color: getCssVar('--color-primary-subdued-1'),
					textColor: getCssVar('--color-primary-hover-1'),
				},
				Dropdown: {
					borderRadius: '12px',
					color: getCssVar('--color-bg-1'),
				},
				Popover: {
					borderRadius: '12px',
					color: getCssVar('--color-bg-1'),
				},
			}
		}

		const themeOverrides = ref<GlobalThemeOverrides>(getThemeOverrides())

		const setTheme = (val: 'light' | 'dark') => {
			theme.value = val
		}

		const changeTheme = () => {
			const isDarkMode = theme.value === 'dark'
			document.documentElement.setAttribute('theme-mode', isDarkMode ? 'dark' : '')
			nextTick(() => {
				themeOverrides.value = getThemeOverrides()
			})
		}

		return {
			theme,
			themeOverrides,
			setTheme,
			changeTheme,
		}
	},
	{
		persist: {
			pick: ['theme'],
		},
	}
)
