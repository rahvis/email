<template>
	<n-tabs v-model:value="menu" class="bt-tabs h-full" type="card" placement="top">
		<slot></slot>
	</n-tabs>
</template>

<script lang="ts" setup>
const menu = defineModel<string | number>('value')
</script>

<style lang="scss" scoped>
.bt-tabs {
	--n-tab-gap: 4px;
	--n-tab-text-color-active: var(--color-text-1);
	--n-tab-border-color: var(--color-border-1);
	--n-tab-padding-vertical: 0 16px;
	--n-pane-padding-top: 16px;
	--n-pane-padding-left: 0;
	--n-pane-padding-right: 0;
	--n-pane-padding-bottom: 0;
	:deep(> .n-tabs-nav) {
		.n-tabs-wrapper {
			.n-tabs-tab-wrapper {
				.n-tabs-tab {
					height: 34px;
					border-radius: var(--radius-pill);
					border-bottom: none;
					background: var(--color-bg-2);
					&--active {
						background: var(--color-bg-1);
					}
				}
			}
		}
	}
	:deep(> .n-tab-pane) {
		flex: 1;
		overflow: auto;
	}
}
</style>
