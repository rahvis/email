import { createDiscreteApi } from 'naive-ui'

const { loadingBar } = createDiscreteApi(['loadingBar'], {
	configProviderProps: {
		themeOverrides: {
			LoadingBar: {
				colorLoading: '#533afd',
				colorError: '#ea2261',
			},
		},
	},
})

export default loadingBar
