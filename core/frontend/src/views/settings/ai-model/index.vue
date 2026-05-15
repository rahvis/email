<template>
	<div class="ai-model-page app-card">
		<div class="ai-model-header">
			<div class="header-title">
				<i class="i-ri:apps-fill text-6"></i>
				<span>AI Model</span>
			</div>
		</div>
		<ModelManager />
	</div>
</template>

<script setup lang="tsx">
import ModelManager from './components/ModelManager.vue'
import { ModelStore } from './dto'
import { useModelManagerStore } from './store/index,'
const modelManagerStore = useModelManagerStore()
provide<ModelStore>('modelStore', modelManagerStore)
</script>

<style scoped lang="scss">
@use '@/styles/index.scss' as base;

.ai-model-page {
	width: 100%;
	padding: var(--space-xl);
	box-sizing: border-box;
}

.ai-model-header {
	padding-bottom: var(--space-lg);
	margin-bottom: var(--space-lg);
	border-bottom: 1px solid var(--color-border-2);
}

.header-title {
	@include base.row-flex-start;
	gap: var(--space-sm);
	align-items: center;
	font-size: 16px;
	font-weight: 700;
	color: var(--color-text-1);
}
</style>
