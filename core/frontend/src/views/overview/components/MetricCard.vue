<template>
	<n-card class="metric-card" :bordered="false">
		<div class="title">{{ title }}</div>
		<div class="value" :style="{ color: textColor }">{{ value }}{{ unit }}</div>
	</n-card>
</template>

<script setup lang="ts">
defineProps({
	title: {
		type: String,
		default: '',
	},
	value: {
		type: Number,
		default: 0,
	},
	unit: {
		type: String,
		default: '',
	},
	textColor: {
		type: String,
	},
})
</script>

<style lang="scss" scoped>
.metric-card {
	--n-padding-top: 24px;
	--n-padding-bottom: 24px;
	--n-text-color: var(--color-card-text-1);
}

.title {
	margin-bottom: 12px;
	color: var(--color-text-3);
	font-size: 13px;
	font-weight: 400;
	letter-spacing: -0.39px;
}

.value {
	font-size: 26px;
	font-weight: 300;
	letter-spacing: -0.26px;
	font-feature-settings: 'tnum', 'ss01';
}
</style>
