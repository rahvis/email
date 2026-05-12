import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { isDev } from '@/utils'

// Static imports for route modules — rspack does not evaluate
// `import.meta.webpackContext` correctly in this build, so list explicitly.
import apiRoute from '@/router/modules/api'
import automationRoute from '@/router/modules/automation'
import contactsRoute from '@/router/modules/contacts'
import domainRoute from '@/router/modules/domain'
import logsRoute from '@/router/modules/logs'
import mailboxRoute from '@/router/modules/mailbox'
import marketRoute from '@/router/modules/market'
import overviewRoute from '@/router/modules/overview'
import settingsRoute from '@/router/modules/settings'
import smtpRoute from '@/router/modules/smtp'
import templateRoute from '@/router/modules/template'
import videoOutreachRoute from '@/router/modules/video-outreach'

// Routes reflect list — controls display order in the sidebar.
const routesReflectList = [
	'Overview',
	'Email Marketing',
	'template',
	'Send API',
	'Contacts',
	'Sequences',
	'Leads',
	'Enrichment',
	'MailDomain',
	'MailBoxes',
	'SMTP',
	'Logs',
	'Settings',
	'Automation',
	'Video Outreach',
]

const rawMenuList: RouteRecordRaw[] = [
	apiRoute,
	automationRoute,
	contactsRoute,
	domainRoute,
	logsRoute,
	mailboxRoute,
	marketRoute,
	overviewRoute,
	settingsRoute,
	smtpRoute,
	templateRoute,
	videoOutreachRoute,
]

// Sort module routes by routesReflectList; preserve any unknowns at the end.
export let menuList: RouteRecordRaw[] = rawMenuList
	.slice()
	.sort((a, b) => {
		const ai = routesReflectList.findIndex(t => t === a.meta?.title)
		const bi = routesReflectList.findIndex(t => t === b.meta?.title)
		const aw = ai === -1 ? Number.MAX_SAFE_INTEGER : ai
		const bw = bi === -1 ? Number.MAX_SAFE_INTEGER : bi
		return aw - bw
	})

const otherArray: RouteRecordRaw[] = []

if (isDev) {
	otherArray.push({
		path: '/test',
		name: 'Test',
		component: () => import('@/views/test/index.vue'),
	})
}

export const routes: RouteRecordRaw[] = [
	{
		path: '/login',
		name: 'Login',
		component: () => import('@/views/login/index.vue'),
	},
	{
		path: '/',
		redirect: '/overview',
	},
	...menuList,
	...otherArray,
]

const router = createRouter({
	history: createWebHistory('/'),
	routes,
	strict: false,
	scrollBehavior: () => ({ left: 0, top: 0 }),
})

export default router
