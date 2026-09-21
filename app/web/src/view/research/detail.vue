<template>
  <div class="zhigu-page-fill">
    <ResearchChatPane />
  </div>
</template>
<script setup>
import { onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import ResearchChatPane from '../../components/research/ResearchChatPane.vue'
import { useResearchConversation } from '../../stores/researchConversation.js'

const route = useRoute()
const conversation = useResearchConversation()

async function hydrate() {
  await conversation.loadRun(route.params.id)
}

onMounted(hydrate)
watch(() => route.params.id, hydrate)
onBeforeUnmount(() => conversation.stopPolling())
</script>
