<template>
	<div class="signup-container stripe-mesh">
		<router-link class="signup-brand" to="/">
			<img src="@/assets/images/logo.png" alt="" />
			<span>{{ BRAND.name }}</span>
		</router-link>

		<div class="signup-card">
			<div class="logo-container">
				<img class="logo" src="@/assets/images/logo.png" alt="" />
			</div>

			<h1 class="signup-title">Create your {{ BRAND.name }} account</h1>
			<p class="signup-subtitle">Start managing your email infrastructure from one dashboard.</p>

			<n-form ref="formRef" size="large" :model="form" :rules="rules">
				<n-form-item :show-label="false" path="username">
					<n-input
						v-model:value="form.username"
						placeholder="Username"
						:input-props="{ spellcheck: false }" />
				</n-form-item>
				<n-form-item :show-label="false" path="email">
					<n-input
						v-model:value="form.email"
						placeholder="Work email"
						:input-props="{ spellcheck: false }" />
				</n-form-item>
				<n-form-item :show-label="false" path="password">
					<n-input
						v-model:value="form.password"
						type="password"
						show-password-on="click"
						placeholder="Password"
						@keyup.enter="handleSignup" />
				</n-form-item>
				<n-form-item :show-label="false" path="confirm_password">
					<n-input
						v-model:value="form.confirm_password"
						type="password"
						show-password-on="click"
						placeholder="Confirm password"
						@keyup.enter="handleSignup" />
				</n-form-item>
				<n-form-item :show-label="false" :show-feedback="false">
					<n-button
						type="primary"
						size="large"
						class="signup-submit"
						:loading="loading"
						:disabled="loading"
						:block="true"
						@click="handleSignup">
						Sign up
					</n-button>
				</n-form-item>
			</n-form>

			<p class="signin-link">
				Already have an account?
				<router-link to="/login">Log in</router-link>
			</p>
		</div>
	</div>
</template>

<script lang="ts" setup>
import { signup } from '@/api/modules/user'
import { BRAND } from '@/config/brand'
import { useUserStore } from '@/store'
import { isObject } from '@/utils'
import {
	isValidSignupEmail,
	isValidSignupUsername,
	signupValidationMessages,
	type SignupFormModel,
} from './validation'

const router = useRouter()
const userStore = useUserStore()
const formRef = useTemplateRef('formRef')
const loading = ref(false)

const form = reactive<SignupFormModel>({
	username: '',
	email: '',
	password: '',
	confirm_password: '',
})

const rules = {
	username: {
		required: true,
		trigger: ['blur', 'input'],
		validator: () => isValidSignupUsername(form.username),
		message: signupValidationMessages.username,
	},
	email: {
		required: true,
		trigger: ['blur', 'input'],
		validator: () => isValidSignupEmail(form.email),
		message: signupValidationMessages.email,
	},
	password: {
		required: true,
		trigger: ['blur', 'input'],
		validator: () => form.password.length >= 8,
		message: signupValidationMessages.password,
	},
	confirm_password: {
		required: true,
		trigger: ['blur', 'input'],
		validator: () => form.password === form.confirm_password,
		message: signupValidationMessages.confirmPassword,
	},
}

interface AuthResponse {
	token: string
	refreshToken?: string
	refresh_token?: string
	ttl: number
}

const handleSignup = async () => {
	try {
		await formRef.value?.validate()
		loading.value = true
		const params = {
			username: form.username.trim(),
			email: form.email.trim(),
			password: form.password,
			confirm_password: form.confirm_password,
		}
		const res = await signup(params)
		if (isObject<AuthResponse>(res)) {
			userStore.setLoginInfo({
				token: res.token,
				refresh_token: res.refresh_token || res.refreshToken || '',
				ttl: res.ttl,
			})
			router.push('/overview')
		}
	} finally {
		loading.value = false
	}
}
</script>

<style lang="scss" scoped>
.signup-container {
	display: flex;
	position: relative;
	justify-content: center;
	align-items: center;
	min-height: 100%;
	padding: 96px 24px 48px;
	background: var(--color-bg-1);
	overflow: hidden;
}

.signup-brand {
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

.signup-card {
	width: 100%;
	max-width: 460px;
	padding: 44px 36px 34px;
	border: 1px solid rgba(255, 255, 255, 0.72);
	border-radius: var(--radius-lg);
	background-color: var(--color-bg-1);
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

.signup-title {
	margin: 0 0 8px;
	text-align: center;
	color: var(--color-text-1);
	font-size: 32px;
	font-weight: 300;
	line-height: 1.1;
}

.signup-subtitle {
	margin: 0 0 28px;
	text-align: center;
	color: var(--color-text-3);
	font-size: 15px;
	line-height: 1.45;
}

.signup-submit {
	margin-top: 4px;
}

.signin-link {
	margin: 20px 0 0;
	text-align: center;
	color: var(--color-text-3);
	font-size: 14px;

	a {
		color: var(--color-primary-1);
		font-weight: 500;
	}
}

@media (max-width: 640px) {
	.signup-container {
		padding: 88px 16px 32px;
	}

	.signup-brand {
		left: 18px;
	}

	.signup-card {
		padding: 36px 22px 30px;
	}
}
</style>
