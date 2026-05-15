<template>
	<modal :title="title" width="560">
		<div class="max-h-640px pt-8px overflow-auto">
			<bt-form ref="formRef" :model="form" :rules="rules" class="pr-12px">
				<n-form-item :label="$t('api.form.apiName')" path="api_name">
					<n-input v-model:value="form.api_name" :placeholder="$t('api.form.apiNamePlaceholder')"></n-input>
				</n-form-item>
				<n-form-item :label="$t('market.task.edit.from')" path="addresser">
					<from-select v-model:value="form.addresser" v-model:domain="form.domain" v-model:name="form.full_name">
					</from-select>
				</n-form-item>
				<n-form-item :label="$t('market.task.edit.displayName')" path="full_name">
					<n-input v-model:value="form.full_name" :placeholder="$t('market.task.edit.displayNamePlaceholder')">
					</n-input>
				</n-form-item>
				<n-form-item :label="$t('market.task.edit.subject')" path="subject">
					<n-input v-model:value="form.subject" :placeholder="$t('market.task.edit.subjectPlaceholder')">
					</n-input>
				</n-form-item>
				<n-form-item :label="$t('market.task.edit.template')" path="template_id">
					<div class="flex-1">
						<template-select v-model:value="form.template_id" v-model:content="form.template_content">
						</template-select>
					</div>
					<n-button text type="primary" class="ml-12px" @click="handleEditTemplate">
						{{ $t('common.actions.edit') }}
					</n-button>
					<n-button text type="primary" class="ml-12px" @click="handlePreviewTemplate(form.template_content)">
						{{ $t('common.actions.preview') }}
					</n-button>
				</n-form-item>
				<n-form-item :label="$t('api.form.status')">
					<n-switch v-model:value="form.active" :checked-value="1" :unchecked-value="0"></n-switch>
				</n-form-item>
				<bt-more>
					<template #title>
						<span>{{ $t('api.form.advancedFeatures') }}</span>
					</template>
					<n-form-item :label="$t('market.task.edit.unsubscribeLink')">
						<n-switch v-model:value="form.unsubscribe" :checked-value="1" :unchecked-value="0">
						</n-switch>
					</n-form-item>
					<n-form-item :label="t('market.task.edit.trackClick')" path="track_click">
						<n-switch v-model:value="form.track_click" :checked-value="1" :unchecked-value="0">
						</n-switch>
					</n-form-item>
					<n-form-item :label="t('market.task.edit.trackOpen')" path="track_open">
						<n-switch v-model:value="form.track_open" :checked-value="1" :unchecked-value="0">
						</n-switch>
					</n-form-item>
					<n-form-item :label="t('market.task.edit.deliveryEngine')" path="delivery_engine">
						<n-radio-group v-model:value="form.delivery_engine">
							<n-space>
								<n-radio value="postfix">{{ t('market.task.edit.enginePostfix') }}</n-radio>
								<n-radio value="kumomta">{{ t('market.task.edit.engineKumo') }}</n-radio>
								<n-radio value="tenant_default">{{ t('market.task.edit.engineTenantDefault') }}</n-radio>
							</n-space>
						</n-radio-group>
					</n-form-item>
					<n-form-item v-if="form.delivery_engine === 'kumomta'" :label="t('market.task.edit.sendingProfile')" path="sending_profile_id">
						<n-select
							v-model:value="form.sending_profile_id"
							:options="sendingProfileOptions"
							:loading="sendingProfilesLoading"
							class="w-260px"
							:placeholder="t('market.task.edit.sendingProfilePlaceholder')" />
						<span class="ml-10px text-12px text-gray-400">{{ t('market.task.edit.sendingProfileTip') }}</span>
					</n-form-item>
					<n-form-item :label="$t('api.form.ipWhitelist')">
						<n-input v-model:value="form.ip_whitelist" :placeholder="$t('api.form.ipWhitelistPlaceholder')">
						</n-input>
					</n-form-item>
					<n-form-item>
						<template #label>
							<div class="flex items-center justify-start gap-1.25">
								<span>{{ $t('api.form.group.label') }}</span>
								<n-popover trigger="hover">
									<template #trigger>
										<i class="i-custom:help"></i>
									</template>
									<bt-tips>
										<li>{{ $t('api.form.group.tips.choose') }}</li>
										<li>{{ $t('api.form.group.tips.autoCreate') }}</li>
									</bt-tips>
								</n-popover>
							</div>
						</template>
						<group-select v-model:value="form.group_id" :show-create="false" clearable>
						</group-select>
					</n-form-item>
				</bt-more>
			</bt-form>
		</div>

		<form-modal />
		<preview-modal />
	</modal>
</template>

<script lang="ts" setup>
import { FormRules } from 'naive-ui'
import { isObject } from '@/utils'
import { useModal } from '@/hooks/modal/useModal'
import { createApi, updateApi } from '@/api/modules/api'
import { getTemplateDetails } from '@/api/modules/market/template'
import { getSendingProfiles, type SendingProfile } from '@/api/modules/tenants'
import { useTenantStore } from '@/store'
import type { Template } from '@/views/template/interface'
import type { Api } from '../types/base'

import TemplateForm from '@/views/template/components/TemplateForm.vue'
import FromSelect from '@/views/market/task/components/FromSelect.vue'
import GroupSelect from '@/views/market/task/components/GroupSelect.vue'
import TemplateSelect from '@/views/market/task/components/TemplateSelect.vue'
import TemplatePreview from '@/views/market/template/components/TemplatePreview.vue'


const { t } = useI18n()
const tenantStore = useTenantStore()

const isEdit = ref(false)

const title = computed(() => {
	return isEdit.value ? t('api.form.title.edit') : t('api.form.title.add')
})

const formRef = useTemplateRef('formRef')

const form = reactive({
	id: 0,
	api_name: '',
	template_id: null as number | null,
	template_content: '',
	subject: '',
	domain: null as string | null,
	addresser: null as string | null,
	full_name: '',
	unsubscribe: 1,
	active: 1,
	ip_whitelist: '',
	track_click: 1,
	track_open: 1,
	delivery_engine: 'postfix' as 'tenant_default' | 'kumomta' | 'postfix',
	sending_profile_id: 0,
	group_id: null as number | null,
})

const sendingProfiles = ref<SendingProfile[]>([])
const sendingProfilesLoading = ref(false)
const sendingProfileOptions = computed(() =>
	sendingProfiles.value
		.filter(profile => profile.status === 'ready' || profile.status === 'warming')
		.map(profile => ({
			label: `${profile.name} (${profile.kumo_pool_name || profile.egress_mode})`,
			value: profile.id,
		}))
)

const loadSendingProfiles = async () => {
	if (!tenantStore.currentTenantID) return
	sendingProfilesLoading.value = true
	try {
		const res = await getSendingProfiles(tenantStore.currentTenantID)
		sendingProfiles.value = Array.isArray(res?.profiles) ? res.profiles : []
	} finally {
		sendingProfilesLoading.value = false
	}
}

watch(
	() => tenantStore.currentTenantID,
	() => {
		form.sending_profile_id = 0
		loadSendingProfiles()
	}
)

const rules: FormRules = {
	api_name: {
		required: true,
		message: t('api.form.validation.apiNameRequired'),
	},
	addresser: {
		required: true,
		message: t('api.form.validation.addresserRequired'),
	},
	full_name: {
		required: true,
		message: t('market.task.edit.displayNamePlaceholder'),
	},
	subject: {
		required: true,
		message: t('market.task.edit.subjectPlaceholder'),
	},
	template_id: {
		required: true,
		message: t('api.form.validation.templateRequired'),
	},
}

const [PreviewModal, previewModalApi] = useModal({
	component: TemplatePreview,
})

const handlePreviewTemplate = (html: string) => {
	previewModalApi.setState({ html })
	previewModalApi.open()
}

const handleEditTemplate = async () => {
	const res = await getTemplateDetails({ id: `${form.template_id}` })
	if (isObject<Template>(res)) {
		if (res.add_type == 2 && res.chat_id) {
			window.open(`/template/ai-template/${res.chat_id}`)
		} else {
			formModalApi.setState({ isEdit: true, row: res })
			formModalApi.open()
		}
	}
}

const [FormModal, formModalApi] = useModal({
	component: TemplateForm,
	state: {
		isEdit: true,
		refresh: async () => {
			const res = await getTemplateDetails({ id: `${form.template_id}` })
			if (isObject<Template>(res)) {
				form.template_content = res.html_content
			}
		},
		preview: (html: string) => {
			handlePreviewTemplate(html)
		},
	},
})

const resetForm = () => {
	form.id = 0
	form.api_name = ''
	form.template_id = null
	form.template_content = ''
	form.subject = ''
	form.addresser = null
	form.full_name = ''
	form.unsubscribe = 1
	form.active = 1
	form.ip_whitelist = ''
	form.track_click = 1
	form.track_open = 1
	form.delivery_engine = 'postfix'
	form.sending_profile_id = 0
	form.group_id = null
}

const getParams = () => {
	return {
		api_name: form.api_name,
		template_id: form.template_id || 0,
		subject: form.subject,
		addresser: form.addresser || '',
		full_name: form.full_name,
		unsubscribe: form.unsubscribe,
		active: form.active,
		ip_whitelist: form.ip_whitelist.split(','),
		track_click: form.track_click,
		track_open: form.track_open,
		delivery_engine: form.delivery_engine,
		sending_profile_id: form.sending_profile_id || 0,
		group_id: form.group_id || 0,
	}
}

const [Modal, modalApi] = useModal({
	onChangeState: isOpen => {
		if (isOpen) {
			loadSendingProfiles()
			const state = modalApi.getState<{ isEdit: boolean; row: Api }>()
			const { row } = state
			isEdit.value = state.isEdit
			if (row) {
				form.id = row.id
				form.api_name = row.api_name
				form.template_id = row.template_id
				form.domain = row.addresser.split('@')[1]
				form.subject = row.subject
				form.addresser = row.addresser
				form.full_name = row.full_name
				form.unsubscribe = row.unsubscribe
				form.active = row.active
				form.ip_whitelist = row.ip_whitelist.join(',')
				form.track_click = row.track_click
				form.track_open = row.track_open
				form.delivery_engine = row.delivery_engine || 'postfix'
				form.sending_profile_id = row.sending_profile_id || 0
				form.group_id = row.group_id || null
			}
		} else {
			resetForm()
		}
	},
	onConfirm: async () => {
		await formRef.value?.validate()
		const params = getParams()
		if (isEdit.value) {
			await updateApi({ ...params, id: form.id })
		} else {
			await createApi(params)
		}
		const state = modalApi.getState<{ refresh: Function }>()
		state.refresh()
	},
})
</script>
