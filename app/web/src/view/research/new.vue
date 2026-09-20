<template>
  <div class="zhigu-page-fill">
    <ResearchChatPane />
  </div>
</template>
<script setup>
import { onMounted } from 'vue'
import { useRoute } from 'vue-router'
import ResearchChatPane from '../../components/research/ResearchChatPane.vue'
import { useResearchConversation } from '../../stores/researchConversation.js'

const route = useRoute()
const conversation = useResearchConversation()
onMounted(() => {
  conversation.prepareNew(route.query)
})
</script>
