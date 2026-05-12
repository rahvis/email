export interface SignupFormModel {
	username: string
	email: string
	password: string
	confirm_password: string
}

export const signupUsernamePattern = /^[A-Za-z0-9_]{4,32}$/

export const signupValidationMessages = {
	username: 'Username must be 4-32 characters and contain only letters, numbers, or underscores.',
	email: 'Enter a valid email address.',
	password: 'Password must be at least 8 characters.',
	confirmPassword: 'Passwords do not match.',
}

export const isValidSignupUsername = (value: string) => signupUsernamePattern.test(value.trim())

export const isValidSignupEmail = (value: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim())

export const validateSignupForm = (form: SignupFormModel): string[] => {
	const errors: string[] = []

	if (!isValidSignupUsername(form.username)) {
		errors.push(signupValidationMessages.username)
	}
	if (!isValidSignupEmail(form.email)) {
		errors.push(signupValidationMessages.email)
	}
	if (form.password.length < 8) {
		errors.push(signupValidationMessages.password)
	}
	if (form.password !== form.confirm_password) {
		errors.push(signupValidationMessages.confirmPassword)
	}

	return errors
}
