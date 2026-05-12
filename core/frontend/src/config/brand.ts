export const PRODUCT_NAME = 'PING2'
export const BRAND_SITE_URL = 'https://ping2.email'
export const BRAND_GITHUB_URL = 'https://github.com/rahvis/email'

export const BRAND = {
	name: PRODUCT_NAME,
	title: PRODUCT_NAME,
	tagline: 'Email infrastructure and marketing operations in one control plane.',
	siteUrl: BRAND_SITE_URL,
	docsUrl: `${BRAND_SITE_URL}/start`,
	githubUrl: BRAND_GITHUB_URL,
	issuesUrl: `${BRAND_GITHUB_URL}/issues`,
	releasesUrl: `${BRAND_GITHUB_URL}/releases`,
}

export const getAppTitle = (suffix?: string) => {
	return suffix ? `${suffix} · ${PRODUCT_NAME}` : PRODUCT_NAME
}
