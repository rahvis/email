<template>
	<main class="landing-page">
		<section class="landing-hero stripe-mesh">
			<nav class="landing-nav">
				<a class="landing-brand" href="/">
					<img src="@/assets/images/logo.png" alt="" />
					<span>{{ BRAND.name }}</span>
				</a>
				<div class="landing-nav-links">
					<a href="#platform">Platform</a>
					<a href="#deliverability">Deliverability</a>
					<a href="#insights">Insights</a>
				</div>
				<div class="landing-nav-actions">
					<router-link class="landing-link" to="/login">Log in</router-link>
					<router-link v-if="!userStore.isLogin" class="landing-link" to="/signup"
						>Sign up</router-link
					>
					<n-button type="primary" @click="goToApp">{{ heroCta }}</n-button>
				</div>
			</nav>

			<div class="landing-container hero-grid">
				<div class="hero-copy">
					<div class="pill-tag-soft">Email operations</div>
					<h1 class="display-xxl">Run high-volume email with product-grade control.</h1>
					<p>
						{{ BRAND.name }} brings mailbox management, campaign execution, delivery diagnostics,
						and API sending into a single Stripe-inspired workspace for teams that need speed and
						visibility.
					</p>
					<div class="hero-actions">
						<n-button type="primary" size="large" @click="goToApp">{{ heroCta }}</n-button>
						<router-link class="stripe-link" :to="userStore.isLogin ? '/overview' : '/login'">
							{{ userStore.isLogin ? 'Open console' : 'Log in' }}
						</router-link>
					</div>
				</div>

				<div class="dashboard-mockup hero-mockup" aria-label="PING2 dashboard preview">
					<div class="mockup-topbar">
						<span></span>
						<span></span>
						<span></span>
						<strong>{{ BRAND.name }}</strong>
					</div>
					<div class="mockup-body">
						<div class="mockup-sidebar">
							<div v-for="item in navItems" :key="item" class="mockup-nav-item">{{ item }}</div>
						</div>
						<div class="mockup-main">
							<div class="mockup-metrics">
								<div v-for="metric in metrics" :key="metric.label" class="mockup-card">
									<span>{{ metric.label }}</span>
									<strong>{{ metric.value }}</strong>
								</div>
							</div>
							<div class="mockup-chart">
								<div v-for="bar in bars" :key="bar" :style="{ height: `${bar}%` }"></div>
							</div>
							<div class="mockup-table">
								<div v-for="row in tableRows" :key="row.domain" class="mockup-row">
									<span>{{ row.domain }}</span>
									<span>{{ row.sent }}</span>
									<span>{{ row.rate }}</span>
								</div>
							</div>
						</div>
					</div>
				</div>
			</div>
		</section>

		<section id="platform" class="landing-section">
			<div class="landing-container feature-grid">
				<div class="feature-card">
					<div class="pill-tag-soft">Control</div>
					<h2 class="display-lg">A clean command center for every sender.</h2>
					<p>
						Manage domains, mailboxes, SMTP relays, templates, subscribers, and API keys without
						switching between operational tools.
					</p>
				</div>
				<div class="feature-card cream">
					<div class="pill-tag-soft">Precision</div>
					<h2 class="display-lg">Numbers stay readable under pressure.</h2>
					<p>
						Tables, rates, and queues use tabular figures so operators can scan delivery,
						engagement, and failure patterns quickly.
					</p>
				</div>
				<div class="feature-card">
					<div class="pill-tag-soft">Flow</div>
					<h2 class="display-lg">Campaign work moves from build to send.</h2>
					<p>
						Compose templates, group audiences, launch tasks, and inspect status details in the same
						dashboard track.
					</p>
				</div>
			</div>
		</section>

		<section id="deliverability" class="landing-dark">
			<div class="landing-container dark-grid">
				<div>
					<div class="pill-tag-soft">Deliverability</div>
					<h2 class="display-xl">Signals for the parts of email that usually stay hidden.</h2>
					<p>
						Watch provider performance, blacklist checks, DNS state, SSL status, delayed queues, and
						failed sends from one dark-app dashboard surface.
					</p>
				</div>
				<div class="console-panel">
					<div class="console-line"><span>delivery_rate</span><strong>98.4%</strong></div>
					<div class="console-line"><span>open_rate</span><strong>42.7%</strong></div>
					<div class="console-line"><span>delayed_queue</span><strong>12</strong></div>
					<div class="console-line active"><span>dns_status</span><strong>verified</strong></div>
				</div>
			</div>
		</section>

		<section id="insights" class="landing-section">
			<div class="landing-container final-band">
				<div>
					<h2 class="display-xl">Bring your sending stack into {{ BRAND.name }}.</h2>
					<p>
						Start from the console, connect domains and senders, then use the dashboard to keep
						campaign and infrastructure work aligned.
					</p>
				</div>
				<n-button type="primary" size="large" @click="goToApp">{{ heroCta }}</n-button>
			</div>
		</section>

		<footer class="landing-footer">
			<div class="landing-container">
				<span>{{ BRAND.name }}</span>
				<span>Built for email infrastructure operators.</span>
			</div>
		</footer>
	</main>
</template>

<script lang="ts" setup>
import { BRAND } from '@/config/brand'
import { useUserStore } from '@/store'

const router = useRouter()
const userStore = useUserStore()

const heroCta = computed(() => (userStore.isLogin ? 'Go to dashboard' : 'Sign up'))

const goToApp = () => {
	router.push(userStore.isLogin ? '/overview' : '/signup')
}

const navItems = ['Overview', 'Send API', 'Domains', 'Mailboxes']

const metrics = [
	{ label: 'Delivered', value: '98.4%' },
	{ label: 'Opened', value: '42.7%' },
	{ label: 'Clicked', value: '11.8%' },
]

const bars = [42, 58, 66, 48, 74, 92, 71, 86, 63, 78, 96, 88]

const tableRows = [
	{ domain: 'acme.com', sent: '42,918', rate: '99.1%' },
	{ domain: 'northwind.io', sent: '18,306', rate: '98.7%' },
	{ domain: 'orbit.dev', sent: '9,734', rate: '97.8%' },
]
</script>

<style lang="scss" scoped>
.landing-page {
	min-height: 100%;
	background: var(--color-bg-1);
	color: var(--color-text-1);
}

.landing-container {
	width: min(1180px, calc(100% - 48px));
	margin: 0 auto;
}

.landing-hero {
	min-height: 760px;
	padding: 24px 0 96px;
}

.landing-nav {
	position: relative;
	z-index: 2;
	display: grid;
	grid-template-columns: auto 1fr auto;
	align-items: center;
	gap: 24px;
	width: min(1180px, calc(100% - 48px));
	margin: 0 auto;
	padding: 16px 24px;
	border: 1px solid rgba(255, 255, 255, 0.72);
	border-radius: var(--radius-xs);
	background: rgba(255, 255, 255, 0.82);
	box-shadow: var(--shadow-level-1);
	backdrop-filter: blur(18px);
}

.landing-brand,
.landing-nav-links,
.landing-nav-actions,
.hero-actions {
	display: flex;
	align-items: center;
}

.landing-brand {
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

.landing-nav-links {
	justify-content: center;
	gap: 28px;

	a {
		color: var(--color-text-2);
		font-size: 15px;
	}
}

.landing-nav-actions {
	justify-content: flex-end;
	gap: 16px;
}

.landing-link {
	color: var(--color-text-2);
	font-weight: 400;
}

.hero-grid {
	display: grid;
	grid-template-columns: minmax(0, 0.9fr) minmax(520px, 1.1fr);
	align-items: center;
	gap: 56px;
	padding-top: 104px;
}

.hero-copy {
	p {
		max-width: 620px;
		margin: 24px 0;
		color: var(--color-text-2);
		font-size: 18px;
		line-height: 1.55;
	}
}

.hero-actions {
	gap: 18px;
}

.hero-mockup {
	overflow: hidden;
	transform: rotate(-1.2deg);
}

.mockup-topbar {
	display: flex;
	align-items: center;
	gap: 7px;
	height: 44px;
	padding: 0 18px;
	border-bottom: 1px solid var(--color-border-1);
	background: var(--color-bg-2);

	span {
		width: 10px;
		height: 10px;
		border-radius: 50%;
		background: var(--color-ruby-1);

		&:nth-child(2) {
			background: var(--color-warning-1);
		}

		&:nth-child(3) {
			background: var(--color-primary-3);
		}
	}

	strong {
		margin-left: auto;
		color: var(--color-text-3);
		font-size: 13px;
		font-weight: 400;
	}
}

.mockup-body {
	display: grid;
	grid-template-columns: 150px 1fr;
	min-height: 420px;
}

.mockup-sidebar {
	padding: 18px 14px;
	background: var(--color-brand-dark-1);
}

.mockup-nav-item {
	margin-bottom: 8px;
	padding: 9px 12px;
	border-radius: var(--radius-pill);
	color: rgba(255, 255, 255, 0.72);
	font-size: 13px;

	&:first-child {
		background: rgba(102, 94, 253, 0.34);
		color: #fff;
	}
}

.mockup-main {
	padding: 22px;
	background: var(--color-bg-1);
}

.mockup-metrics {
	display: grid;
	grid-template-columns: repeat(3, 1fr);
	gap: 12px;
}

.mockup-card {
	padding: 16px;
	border: 1px solid var(--color-border-1);
	border-radius: var(--radius-lg);
	background: var(--color-bg-2);

	span {
		display: block;
		color: var(--color-text-3);
		font-size: 12px;
	}

	strong {
		display: block;
		margin-top: 8px;
		font-size: 24px;
		font-weight: 300;
		letter-spacing: -0.26px;
	}
}

.mockup-chart {
	display: flex;
	align-items: end;
	gap: 10px;
	height: 128px;
	margin: 20px 0;
	padding: 18px;
	border-radius: var(--radius-lg);
	background: linear-gradient(180deg, #f6f9fc, #ffffff);

	div {
		flex: 1;
		min-width: 12px;
		border-radius: var(--radius-pill) var(--radius-pill) 3px 3px;
		background: linear-gradient(180deg, var(--color-primary-3), var(--color-primary-1));
	}
}

.mockup-table {
	border: 1px solid var(--color-border-1);
	border-radius: var(--radius-lg);
	overflow: hidden;
}

.mockup-row {
	display: grid;
	grid-template-columns: 1fr 90px 70px;
	gap: 12px;
	padding: 12px 14px;
	border-bottom: 1px solid var(--color-border-1);
	font-size: 13px;
	font-feature-settings: 'tnum', 'ss01';

	&:last-child {
		border-bottom: 0;
	}

	span:not(:first-child) {
		text-align: right;
		color: var(--color-text-3);
	}
}

.landing-section {
	padding: 88px 0;
	background: var(--color-bg-1);
}

.feature-grid {
	display: grid;
	grid-template-columns: repeat(3, 1fr);
	gap: 24px;
}

.feature-card {
	min-height: 310px;
	padding: 32px;
	border: 1px solid var(--color-border-1);
	border-radius: var(--radius-lg);
	background: var(--color-bg-1);
	box-shadow: var(--shadow-level-1);

	&.cream {
		background: var(--color-bg-cream-1);
	}

	h2 {
		margin: 22px 0 16px;
	}

	p {
		color: var(--color-text-2);
		font-size: 16px;
		line-height: 1.55;
	}
}

.landing-dark {
	padding: 96px 0;
	background: var(--color-brand-dark-1);
	color: var(--color-button-text-1);

	p {
		max-width: 580px;
		margin-top: 22px;
		color: rgba(255, 255, 255, 0.72);
		font-size: 17px;
		line-height: 1.55;
	}
}

.dark-grid {
	display: grid;
	grid-template-columns: minmax(0, 1fr) 420px;
	align-items: center;
	gap: 64px;
}

.console-panel {
	padding: 24px;
	border: 1px solid rgba(255, 255, 255, 0.12);
	border-radius: var(--radius-lg);
	background: rgba(6, 9, 40, 0.58);
	box-shadow: var(--shadow-floating);
}

.console-line {
	display: flex;
	justify-content: space-between;
	padding: 14px 0;
	border-bottom: 1px solid rgba(255, 255, 255, 0.1);
	font-family: var(--font-mono);
	font-size: 13px;

	&:last-child {
		border-bottom: 0;
	}

	span {
		color: rgba(255, 255, 255, 0.56);
	}

	strong {
		color: #fff;
		font-weight: 400;
	}

	&.active strong {
		color: var(--color-primary-3);
	}
}

.final-band {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 32px;
	padding: 40px;
	border-radius: var(--radius-lg);
	background: var(--color-bg-2);

	p {
		max-width: 660px;
		margin-top: 16px;
		color: var(--color-text-2);
		font-size: 17px;
	}
}

.landing-footer {
	padding: 44px 0;
	border-top: 1px solid var(--color-border-1);
	color: var(--color-text-3);
	font-size: 13px;

	.landing-container {
		display: flex;
		justify-content: space-between;
		gap: 24px;
	}
}

@media (max-width: 1023px) {
	.landing-nav {
		grid-template-columns: auto auto;
	}

	.landing-nav-links {
		display: none;
	}

	.hero-grid,
	.dark-grid {
		grid-template-columns: 1fr;
	}

	.hero-grid {
		padding-top: 72px;
	}

	.hero-mockup {
		transform: none;
	}

	.feature-grid {
		grid-template-columns: 1fr;
	}
}

@media (max-width: 767px) {
	.landing-container,
	.landing-nav {
		width: min(100% - 28px, 1180px);
	}

	.landing-hero {
		padding-top: 14px;
	}

	.landing-nav {
		padding: 12px;
	}

	.landing-nav-actions {
		gap: 10px;
	}

	.landing-link {
		display: none;
	}

	.hero-copy p {
		font-size: 16px;
	}

	.mockup-body {
		grid-template-columns: 1fr;
	}

	.mockup-sidebar {
		display: none;
	}

	.mockup-metrics {
		grid-template-columns: 1fr;
	}

	.dark-grid {
		gap: 32px;
	}

	.final-band,
	.landing-footer .landing-container {
		align-items: flex-start;
		flex-direction: column;
	}
}
</style>
