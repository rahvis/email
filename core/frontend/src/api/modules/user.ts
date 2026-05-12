import { instance } from '@/api'
import i18n from '@/i18n'

const { t } = i18n.global

/**
 * @description 登录
 * @param params
 * @returns
 */
export const login = (params: { username: string; password: string }) => {
	return instance.post('/login', params, {
		fetchOptions: {
			successMessage: true,
		},
	})
}

/**
 * @description 注册
 * @param params
 * @returns
 */
export const signup = (params: {
	username: string
	email: string
	password: string
	confirm_password: string
}) => {
	return instance.post('/signup', params, {
		fetchOptions: {
			successMessage: true,
		},
	})
}

/**
 * @description 退出登录
 * @param params
 * @returns
 */
export const logout = (refreshToken?: string) => {
	return instance.post('/logout', refreshToken ? { refreshToken } : {}, {
		fetchOptions: {
			loading: t('user.api.loading.logout'),
			successMessage: true,
		},
	})
}

/**
 * @description 刷新登录令牌
 * @param refreshTokenValue
 * @returns
 */
export const refreshToken = (refreshTokenValue: string) => {
	return instance.post('/refresh-token', { refreshToken: refreshTokenValue })
}

/**
 * @description 获取验证码
 * @param params
 * @returns
 */
export const getValidateCode = () => {
	return instance.get('/get_validate_code')
}
