import { useGlobalStore, useUserStore } from '@/store'
import { setLanguage } from '@/i18n'
import { clearPendingRequests } from '@/api'
import router from '@/router/router'
import loadingBar from '@/config/loadingBar'

// Route white list
const whitePathList = ['/', '/login']

router.beforeEach(async (to, from, next) => {
	loadingBar.start()

	clearPendingRequests()

	const globalStore = useGlobalStore()

	// Set the language
	try {
		await globalStore.getLang()
		setLanguage(globalStore.lang)
	} catch {
		setLanguage(globalStore.lang)
	}

	if (!to.matched.length) {
		next()
		return
	}

	const userStore = useUserStore()

	// User is logged in
	if (userStore.isLogin) {
		// If the visited route is in the white list, jump to the home page
		if (to.path === '/login') {
			next('/overview')
		} else {
			next()
		}
	} else if (whitePathList.includes(to.path)) {
		// If the visited route is in the white list, go directly
		next()
	} else {
		next('/login')
	}
})

router.afterEach(() => {
	loadingBar.finish()
})

export default router
