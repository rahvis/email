<template>
	<n-layout class="app-layout w-full h-full" has-sider>
		<sidebar></sidebar>
		<n-layout class="app-content" :content-style="{ position: 'relative', paddingTop: '56px' }" @scroll="handleScroll">
			<app-header :top="scrollTop"></app-header>
			<app-main></app-main>
		</n-layout>
	</n-layout>
</template>

<script lang="ts" setup>
import { Sidebar, AppHeader, AppMain } from './components'

const scrollTop = ref(0)

const handleScroll = (e: Event) => {
	scrollTop.value = (e.target as HTMLElement).scrollTop || 0
}
</script>

<style lang="scss" scoped>
.app-layout {
	background: var(--color-bg-2);
}

.app-content {
	background:
		linear-gradient(180deg, rgba(255, 255, 255, 0.72), rgba(255, 255, 255, 0) 220px),
		var(--color-bg-2);
}
</style>
