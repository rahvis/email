<template>
	<div class="login-container stripe-mesh">
		<router-link class="login-brand" to="/">
			<img src="@/assets/images/logo.png" alt="" />
			<span>{{ BRAND.name }}</span>
		</router-link>
		<div class="login-card">
			<div class="logo-container">
				<img class="logo" src="@/assets/images/logo.png" alt="" />
			</div>

			<h1 class="login-title">{{ BRAND.name }}</h1>
			<p class="login-subtitle">Sign in to manage your email infrastructure.</p>

			<n-form ref="formRef" size="large" :model="form" :rules="rules">
				<n-form-item :show-label="false" path="username">
					<n-input v-model:value="form.username" :placeholder="t('login.form.usernamePlaceholder')">
					</n-input>
				</n-form-item>
				<n-form-item :show-label="false" path="password">
					<n-input
						v-model:value="form.password"
						class="password-input"
						type="password"
						show-password-on="click"
						:placeholder="t('login.form.passwordPlaceholder')"
						@keyup.enter="handleLogin">
					</n-input>
				</n-form-item>
				<n-form-item v-if="isCode" :show-label="false" path="validate_code">
					<n-input
						v-model:value="form.validate_code"
						class="flex-1"
						:placeholder="t('login.form.captcha')"
						:input-props="{ spellcheck: false }"
						@keydown.enter="handleLogin">
					</n-input>
					<n-spin size="small" :show="codeLoading">
						<div class="code" @click="getCode()">
							<img
								class="w-full h-full"
								:src="codeUrl"
								:title="t('login.form.changeCaptcha')"
								:alt="t('login.form.captcha')" />
						</div>
					</n-spin>
				</n-form-item>
				<n-form-item :show-label="false" :show-feedback="false">
					<n-button
						type="primary"
						size="large"
						class="login-submit"
						:loading="loading"
						:disabled="loading"
						:block="true"
						@click="handleLogin">
						{{ t('login.form.loginButton') }}
					</n-button>
				</n-form-item>
			</n-form>

			<p class="signup-link">
				New to {{ BRAND.name }}?
				<router-link to="/signup">Create an account</router-link>
			</p>
		</div>
	</div>
</template>

<script lang="ts" setup>
import { useUserStore } from '@/store'
import { isObject } from '@/utils'
import { getValidateCode, login } from '@/api/modules/user'
import { BRAND } from '@/config/brand'

const { t } = useI18n()

const router = useRouter()
const userStore = useUserStore()

const formRef = useTemplateRef('formRef')

const isCode = ref(false)

const codeUrl = ref('')

const codeLoading = ref(false)

const form = reactive({
	username: '',
	password: '',
	validate_code: '',
	validate_code_id: '',
})

const rules = {
	username: {
		required: true,
		message: t('login.validation.usernameRequired'),
		trigger: ['blur', 'input'],
	},
	password: {
		required: true,
		message: t('login.validation.passwordRequired'),
		trigger: ['blur', 'input'],
	},
	validate_code: {
		required: true,
		trigger: ['blur', 'input'],
		message: t('login.validation.captchaRequired'),
	},
}

interface CodeResponse {
	mustValidateCode: boolean
	validateCodeBase64: string
	validateCodeId: string
}

const getCode = async () => {
	try {
		codeLoading.value = true
		const res = await getValidateCode()
		if (isObject<CodeResponse>(res)) {
			isCode.value = res.mustValidateCode
			if (res.mustValidateCode) {
				codeUrl.value = res.validateCodeBase64
				form.validate_code_id = res.validateCodeId
			}
		}
	} finally {
		codeLoading.value = false
	}
}

const loading = ref(false)

interface LoginResponse {
	token: string
	refreshToken?: string
	refresh_token?: string
	ttl: number
}

const handleLogin = async () => {
	try {
		await formRef.value?.validate()
		loading.value = true
		const res = await login(toRaw(form))
		if (isObject<LoginResponse>(res)) {
			userStore.setLoginInfo({
				token: res.token,
				refresh_token: res.refresh_token || res.refreshToken || '',
				ttl: res.ttl,
			})
			setTimeout(() => {
				router.push('/overview')
			}, 1000)
		}
	} catch {
		getCode()
	} finally {
		loading.value = false
	}
}

getCode()
</script>

<style lang="scss" scoped>
.login-container {
	display: flex;
	position: relative;
	justify-content: center;
	align-items: center;
	min-height: 100%;
	padding: 96px 24px 48px;
	background: var(--color-bg-1);
	overflow: hidden;
}

.login-brand {
	position: absolute;
	top: 24px;
	left: 32px;
	display: inline-flex;
	align-items: center;
	gap: 10px;
	color: var(--color-text-1);
	font-size: 18px;
	font-weight: 400;

	img {
		width: 32px;
		height: 32px;
		object-fit: contain;
	}
}

.login-card {
	width: 100%;
	max-width: 420px;
	background-color: var(--color-bg-1);
	padding: 44px 36px 40px;
	border: 1px solid rgba(255, 255, 255, 0.72);
	border-radius: var(--radius-lg);
	box-shadow: var(--shadow-floating);
	backdrop-filter: blur(16px);
	z-index: 1;
}

.logo-container {
	display: flex;
	align-items: center;
	justify-content: center;
	margin-bottom: 20px;
}

.logo {
	width: 62px;
	height: 62px;
	object-fit: contain;
}

.login-title {
	margin-top: 0;
	margin-bottom: 8px;
	text-align: center;
	font-size: 32px;
	font-weight: 300;
	line-height: 1.1;
	letter-spacing: -0.64px;
	color: var(--color-text-1);
}

.login-subtitle {
	margin: 0 0 28px;
	text-align: center;
	color: var(--color-text-3);
	font-size: 15px;
}

.login-submit {
	margin-top: 4px;
}

.signup-link {
	margin: 20px 0 0;
	text-align: center;
	color: var(--color-text-3);
	font-size: 14px;

	a {
		color: var(--color-primary-1);
		font-weight: 500;
	}
}

.code {
	width: 120px;
	height: 40px;
	margin-left: 12px;
	border-radius: var(--radius-sm);
	border: 1px solid var(--color-border-2);
	overflow: hidden;
	cursor: pointer;
}

@media (max-width: 640px) {
	.login-container {
		padding: 88px 16px 32px;
	}

	.login-brand {
		left: 18px;
	}

	.login-card {
		padding: 36px 22px 30px;
	}
}
</style>
