const canvas = document.getElementById("map");
const ctx = canvas.getContext("2d");
const runSelect = document.getElementById("runSelect");
const replayButton = document.getElementById("replayButton");
const playPauseButton = document.getElementById("playPauseButton");
const stepButton = document.getElementById("stepButton");
const speedRange = document.getElementById("speedRange");
const stageFilter = document.getElementById("stageFilter");
const focusModeButton = document.getElementById("focusModeButton");
const clearFocusButton = document.getElementById("clearFocusButton");
const eventCounter = document.getElementById("eventCounter");
const liveStatus = document.getElementById("liveStatus");
const agentRoster = document.getElementById("agentRoster");
const conversationLog = document.getElementById("conversationLog");
const timeline = document.getElementById("timeline");
const inspector = document.getElementById("inspector");

const zones = {
  planner: { x: 190, y: 165, label: "Planning desk", color: "#f2d17b" },
  "code-writer": { x: 555, y: 165, label: "Coding table", color: "#88c7f0" },
  tester: { x: 910, y: 165, label: "Testing lab", color: "#d99ddb" },
  documenter: { x: 360, y: 510, label: "Docs library", color: "#b89af0" },
  deployer: { x: 760, y: 510, label: "Deploy gate", color: "#f09b72" },
};

const animalGlyphs = {
  cat: "🐱",
  dog: "🐶",
  rabbit: "🐰",
  fox: "🦊",
  penguin: "🐧",
};

const agents = new Map();
let events = [];
let replayTimer = null;
let animationFrame = null;
let replayIndex = 0;
let isPlaying = false;
let activeStage = "";
let activeAgentName = "";
let selectedStage = "";
let focusedAgentName = "";
let decisionLinks = [];
let conversationItems = [];
let liveStream = null;
let liveEventKeys = new Set();

function resizeCanvas() {
  const rect = canvas.getBoundingClientRect();
  const ratio = window.devicePixelRatio || 1;
  canvas.width = Math.max(900, Math.floor(rect.width * ratio));
  canvas.height = Math.max(540, Math.floor(rect.height * ratio));
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
  draw(performance.now());
}

function scalePoint(point) {
  const rect = canvas.getBoundingClientRect();
  return {
    x: (point.x / 1100) * rect.width,
    y: (point.y / 720) * rect.height,
  };
}

function draw(now) {
  const rect = canvas.getBoundingClientRect();
  ctx.clearRect(0, 0, rect.width, rect.height);
  drawGround(rect);
  Object.entries(zones).forEach(([stage, zone]) => drawZone(stage, zone));
  drawDecisionLinks(now);
  agents.forEach((agent) => drawAgent(agent, now));
}

function drawGround(rect) {
  ctx.fillStyle = "#a8d7b1";
  ctx.fillRect(0, 0, rect.width, rect.height);
  ctx.strokeStyle = "rgba(42, 74, 52, 0.18)";
  ctx.lineWidth = 1;
  for (let x = 0; x < rect.width; x += 36) {
    ctx.beginPath();
    ctx.moveTo(x, 0);
    ctx.lineTo(x, rect.height);
    ctx.stroke();
  }
  for (let y = 0; y < rect.height; y += 36) {
    ctx.beginPath();
    ctx.moveTo(0, y);
    ctx.lineTo(rect.width, y);
    ctx.stroke();
  }
}

function drawZone(stage, zone) {
  const point = scalePoint(zone);
  ctx.fillStyle = zone.color;
  roundRect(point.x - 92, point.y - 52, 184, 104, 8);
  ctx.fill();
  ctx.strokeStyle = stage === activeStage ? "#2e6f50" : "rgba(23, 32, 26, 0.28)";
  ctx.lineWidth = stage === activeStage ? 5 : 1;
  ctx.stroke();
  ctx.lineWidth = 1;
  ctx.fillStyle = "#17201a";
  ctx.font = "700 14px system-ui";
  ctx.textAlign = "center";
  ctx.fillText(zone.label, point.x, point.y + 4);
  ctx.font = "12px system-ui";
  ctx.fillText(stage, point.x, point.y + 24);
}

function drawDecisionLinks(now) {
  decisionLinks = decisionLinks.filter((link) => now - link.createdAt < 2600);
  decisionLinks.forEach((link) => {
    const fromAgent = agents.get(link.from);
    const toAgent = agents.get(link.to);
    if (!fromAgent || !toAgent) {
      return;
    }
    const from = scalePoint(fromAgent.position);
    const to = scalePoint(toAgent.position);
    const age = now - link.createdAt;
    const alpha = Math.max(0, 1 - age / 2600);
    ctx.save();
    ctx.strokeStyle = `rgba(46, 111, 80, ${alpha})`;
    ctx.lineWidth = 4;
    ctx.setLineDash([12, 8]);
    ctx.beginPath();
    ctx.moveTo(from.x, from.y);
    ctx.quadraticCurveTo((from.x + to.x) / 2, Math.min(from.y, to.y) - 80, to.x, to.y);
    ctx.stroke();
    ctx.restore();
  });
}

function drawAgent(agent, now) {
  const unfocused = focusedAgentName && agent.name !== focusedAgentName;
  const point = scalePoint(agent.position);
  const target = scalePoint(agent.targetPosition || agent.position);
  const spawnAge = now - agent.spawnedAt;
  const spawnScale = Math.min(1, easeOutBack(Math.max(0, spawnAge) / 520));
  const moving = agent.moveStartedAt && now - agent.moveStartedAt < agent.moveDuration;

  ctx.save();
  ctx.globalAlpha = unfocused ? 0.34 : 1;

  if (moving) {
    ctx.save();
    ctx.setLineDash([8, 8]);
    ctx.strokeStyle = "rgba(23, 32, 26, 0.28)";
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.moveTo(point.x, point.y);
    ctx.lineTo(target.x, target.y);
    ctx.stroke();
    ctx.restore();
  }

  if (spawnAge < 900) {
    const ringProgress = spawnAge / 900;
    ctx.strokeStyle = `rgba(46, 111, 80, ${1 - ringProgress})`;
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.arc(point.x, point.y, 30 + ringProgress * 28, 0, Math.PI * 2);
    ctx.stroke();
  }

  ctx.save();
  ctx.translate(point.x, point.y);
  ctx.scale(spawnScale, spawnScale);
  ctx.fillStyle = "rgba(0, 0, 0, 0.18)";
  ctx.beginPath();
  ctx.ellipse(0, 32, 32, 10, 0, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = "#fffaf0";
  ctx.beginPath();
  ctx.arc(0, 0, 32, 0, Math.PI * 2);
  ctx.fill();
  ctx.strokeStyle = agent.status === "error" ? "#c24137" : "#2e6f50";
  ctx.lineWidth = 4;
  ctx.stroke();
  ctx.font = "32px serif";
  ctx.textAlign = "center";
  ctx.fillText(animalGlyphs[agent.avatar] || "🐾", 0, 11);
  ctx.restore();

  ctx.fillStyle = "#17201a";
  ctx.font = "700 12px system-ui";
  ctx.fillText(agent.name, point.x, point.y + 54);
  if (agent.message) {
    ctx.fillStyle = "#fffaf0";
    roundRect(point.x - 98, point.y - 84, 196, 42, 8);
    ctx.fill();
    ctx.strokeStyle = "rgba(23, 32, 26, 0.18)";
    ctx.stroke();
    ctx.fillStyle = "#17201a";
    ctx.font = "12px system-ui";
    ctx.fillText(agent.message.slice(0, 28), point.x, point.y - 59);
    if (agent.message.length > 28) {
      ctx.fillText(agent.message.slice(28, 52), point.x, point.y - 45);
    }
  }
  ctx.restore();
}

function animationLoop(now) {
  updateAgentPositions(now);
  draw(now);
  animationFrame = requestAnimationFrame(animationLoop);
}

function updateAgentPositions(now) {
  agents.forEach((agent) => {
    if (!agent.moveStartedAt || !agent.startPosition || !agent.targetPosition) {
      return;
    }
    const progress = Math.min(1, (now - agent.moveStartedAt) / agent.moveDuration);
    const eased = easeInOutCubic(progress);
    agent.position = {
      x: lerp(agent.startPosition.x, agent.targetPosition.x, eased),
      y: lerp(agent.startPosition.y, agent.targetPosition.y, eased),
    };
    if (progress >= 1) {
      agent.moveStartedAt = 0;
      agent.startPosition = null;
      agent.position = { ...agent.targetPosition };
    }
  });
}

function moveAgentTo(agent, targetPosition, now) {
  agent.startPosition = { ...agent.position };
  agent.targetPosition = { ...targetPosition };
  agent.moveStartedAt = now;
  agent.moveDuration = 850;
}

function easeInOutCubic(value) {
  return value < 0.5 ? 4 * value * value * value : 1 - Math.pow(-2 * value + 2, 3) / 2;
}

function easeOutBack(value) {
  const clamped = Math.min(1, value);
  const c1 = 1.70158;
  const c3 = c1 + 1;
  return 1 + c3 * Math.pow(clamped - 1, 3) + c1 * Math.pow(clamped - 1, 2);
}

function lerp(start, end, progress) {
  return start + (end - start) * progress;
}

function roundRect(x, y, width, height, radius) {
  ctx.beginPath();
  ctx.moveTo(x + radius, y);
  ctx.arcTo(x + width, y, x + width, y + height, radius);
  ctx.arcTo(x + width, y + height, x, y + height, radius);
  ctx.arcTo(x, y + height, x, y, radius);
  ctx.arcTo(x, y, x + width, y, radius);
  ctx.closePath();
}

function escapeHTML(value) {
  return String(value || "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

async function loadRuns() {
  const response = await fetch("/api/runs");
  const payload = await response.json();
  runSelect.innerHTML = "";
  payload.runs.forEach((run) => {
    const option = document.createElement("option");
    option.value = run.runId;
    option.textContent = `${run.status} · ${run.request || run.runId}`;
    runSelect.appendChild(option);
  });
  if (payload.runs.length > 0) {
    await loadEvents(payload.runs[0].runId);
  }
}

async function loadEvents(runId) {
  closeLiveStream();
  const response = await fetch(`/api/runs/${runId}/events`);
  const payload = await response.json();
  events = payload.events || [];
  agents.clear();
  replayIndex = 0;
  isPlaying = false;
  activeStage = "";
  activeAgentName = "";
  focusedAgentName = "";
  decisionLinks = [];
  conversationItems = [];
  timeline.innerHTML = "";
  conversationLog.innerHTML = "";
  renderAgentRoster();
  updatePlaybackControls();
  draw(performance.now());
  connectLiveStream(runId);
}

function replay() {
  clearInterval(replayTimer);
  agents.clear();
  replayIndex = 0;
  isPlaying = true;
  activeStage = "";
  activeAgentName = "";
  focusedAgentName = "";
  decisionLinks = [];
  conversationItems = [];
  timeline.innerHTML = "";
  conversationLog.innerHTML = "";
  renderAgentRoster();
  updatePlaybackControls();
  replayTimer = setInterval(stepForward, playbackDelay());
}

function connectLiveStream(runId) {
  closeLiveStream();
  if (!window.EventSource || !runId) {
    updateLiveStatus("Live: unsupported", false);
    return;
  }
  liveEventKeys = new Set(events.map(eventKey));
  updateLiveStatus("Live: connecting", false);
  const cursor = events.length;
  liveStream = new EventSource(`/api/runs/${encodeURIComponent(runId)}/events/stream?follow=1&cursor=${cursor}`);
  liveStream.addEventListener("open", () => updateLiveStatus("Live: connected", true));
  liveStream.addEventListener("error", () => updateLiveStatus("Live: reconnecting", false));
  liveStream.addEventListener("visualization-event", (message) => {
    const event = JSON.parse(message.data);
    const key = eventKey(event);
    if (liveEventKeys.has(key)) {
      return;
    }
    liveEventKeys.add(key);
    events.push(event);
    if (!isPlaying) {
      applyEvent(event);
      replayIndex = events.length;
      updatePlaybackControls();
    }
  });
}

function closeLiveStream() {
  if (!liveStream) {
    return;
  }
  liveStream.close();
  liveStream = null;
}

function updateLiveStatus(label, online) {
  liveStatus.textContent = label;
  liveStatus.classList.toggle("is-offline", !online);
}

function eventKey(event) {
  return [event.runId, event.type, event.stage || "", event.timestamp || "", event.agent ? event.agent.name : ""].join("|");
}

function stepForward() {
  if (replayIndex >= events.length) {
    pausePlayback();
    return;
  }
  applyEvent(events[replayIndex]);
  replayIndex += 1;
  updatePlaybackControls();
  if (replayIndex >= events.length) {
    pausePlayback();
  }
}

function pausePlayback() {
  clearInterval(replayTimer);
  replayTimer = null;
  isPlaying = false;
  updatePlaybackControls();
}

function resumePlayback() {
  if (isPlaying || replayIndex >= events.length) {
    return;
  }
  isPlaying = true;
  updatePlaybackControls();
  replayTimer = setInterval(stepForward, playbackDelay());
}

function playbackDelay() {
  return Number(speedRange.value);
}

function updatePlaybackControls() {
  playPauseButton.textContent = isPlaying ? "Pause" : "Play";
  eventCounter.textContent = `${Math.min(replayIndex, events.length)} / ${events.length} events`;
}

function applyEvent(event) {
  if (event.stage) {
    activeStage = event.stage;
  }
  if (event.agent) {
    const now = performance.now();
    const current = agents.get(event.agent.name) || {
      name: event.agent.name,
      avatar: event.agent.avatar.kind,
      position: spawnPosition(agents.size),
      targetPosition: spawnPosition(agents.size),
      startPosition: null,
      moveStartedAt: 0,
      moveDuration: 0,
      spawnedAt: now,
      status: "active",
      message: "created",
    };
    const zone = zones[event.stage];
    if (zone && event.type !== "agent-spawned") {
      moveAgentTo(current, {
        x: zone.x + ((agents.size % 3) - 1) * 42,
        y: zone.y + 8 + Math.floor(agents.size / 3) * 36,
      }, now);
    }
    if (event.type === "agent-spawned") {
      current.spawnedAt = now;
      current.message = event.message || "Hello, I'm ready.";
    }
    current.status = event.status || current.status;
    if (event.type !== "agent-spawned") {
      current.message = speechForEvent(event);
    }
    current.role = event.agent.role;
    current.metrics = event.metrics;
    current.decision = event.decision;
    current.lastSpokeAt = now;
    activeAgentName = current.name;
    agents.set(event.agent.name, current);
    registerDecisionLink(event, current);
    appendConversation(event, current);
    renderAgentRoster();
    renderInspector(current);
  } else {
    appendConversation(event, null);
  }
  appendTimeline(event);
}

function applyStageFilter(event) {
  if (!selectedStage) {
    return true;
  }
  return event.stage === selectedStage || !event.stage;
}

function setFocusedAgent(name) {
  focusedAgentName = name || "";
  draw(performance.now());
  renderAgentRoster();
}

function registerDecisionLink(event, current) {
  if (!event.decision || !event.decision.existingAgentName) {
    return;
  }
  if (!agents.has(event.decision.existingAgentName)) {
    return;
  }
  decisionLinks.push({
    from: event.decision.existingAgentName,
    to: current.name,
    createdAt: performance.now(),
  });
}

function speechForEvent(event) {
  if (event.message) {
    return event.message;
  }
  switch (event.type) {
    case "stage-started":
      return `Starting ${event.stage}.`;
    case "agent-decision":
      return event.decision ? `Decision: ${event.decision.action}.` : "I made a decision.";
    case "stage-completed":
      return event.status === "success" ? "Done. Passing it on." : "I hit a problem.";
    case "metrics-updated":
      return "Updated my scorecard.";
    default:
      return event.type;
  }
}

function spawnPosition(index) {
  return { x: 105 + index * 56, y: 92 };
}

function appendTimeline(event) {
  if (!applyStageFilter(event)) {
    return;
  }
  const item = document.createElement("li");
  const label = event.agent ? event.agent.name : event.runId;
  item.innerHTML = `<strong>${escapeHTML(event.type)}</strong><span>${escapeHTML(label)}</span><p>${escapeHTML(event.message || event.stage || "")}</p>`;
  timeline.appendChild(item);
  timeline.scrollLeft = timeline.scrollWidth;
}

function appendConversation(event, agent) {
  if (!applyStageFilter(event)) {
    return;
  }
  const speaker = agent ? agent.name : "Spinning Wheel";
  const message = agent ? speechForEvent(event) : event.message || event.type;
  conversationItems.push({
    speaker,
    stage: event.stage || "run",
    type: event.type,
    message,
  });
  conversationItems = conversationItems.slice(-14);
  renderConversation();
}

function renderConversation() {
  conversationLog.innerHTML = conversationItems.map((item) => `
    <li>
      <strong>${escapeHTML(item.speaker)}</strong>
      <span>${escapeHTML(item.stage)} · ${escapeHTML(item.type)}</span>
      <p>${escapeHTML(item.message)}</p>
    </li>
  `).join("");
  conversationLog.scrollTop = conversationLog.scrollHeight;
}

function rebuildFilteredLogs() {
  timeline.innerHTML = "";
  conversationItems = [];
  events.slice(0, replayIndex).forEach((event) => {
    appendTimeline(event);
    appendConversation(event, event.agent ? agents.get(event.agent.name) : null);
  });
}

function renderAgentRoster() {
  if (agents.size === 0) {
    agentRoster.innerHTML = `<li><span>No active agents yet.</span></li>`;
    return;
  }
  agentRoster.innerHTML = Array.from(agents.values()).map((agent) => `
    <li class="${rosterClasses(agent)}" data-agent="${escapeHTML(agent.name)}">
      <strong>${animalGlyphs[agent.avatar] || "🐾"} ${escapeHTML(agent.name)}</strong>
      <span>${escapeHTML(agent.role || "agent")} · ${escapeHTML(agent.status || "active")}</span>
      <span>${escapeHTML(agent.message || "")}</span>
    </li>
  `).join("");
}

function rosterClasses(agent) {
  const classes = [];
  if (agent.name === activeAgentName) {
    classes.push("is-active");
  }
  if (agent.name === focusedAgentName) {
    classes.push("is-focused");
  }
  return classes.join(" ");
}

function renderInspector(agent) {
  inspector.innerHTML = `
    <h2>${escapeHTML(agent.name)}</h2>
    <p>${animalGlyphs[agent.avatar] || "🐾"} ${escapeHTML(agent.role || "agent")}</p>
    <p>Status: ${escapeHTML(agent.status)}</p>
    <p>${escapeHTML(agent.message || "")}</p>
    ${agent.decision ? `<p>Decision: ${escapeHTML(agent.decision.action)} (${escapeHTML(agent.decision.source || "unknown")})</p>` : ""}
    ${agent.metrics ? `<p>Success rate: ${Math.round((agent.metrics.successRate || 0) * 100)}%</p>` : ""}
  `;
}

runSelect.addEventListener("change", () => loadEvents(runSelect.value));
replayButton.addEventListener("click", replay);
playPauseButton.addEventListener("click", () => {
  if (isPlaying) {
    pausePlayback();
  } else {
    resumePlayback();
  }
});
stepButton.addEventListener("click", () => {
  if (isPlaying) {
    pausePlayback();
  }
  stepForward();
});
speedRange.addEventListener("input", () => {
  if (!isPlaying) {
    return;
  }
  clearInterval(replayTimer);
  replayTimer = setInterval(stepForward, playbackDelay());
});
stageFilter.addEventListener("change", () => {
  selectedStage = stageFilter.value;
  rebuildFilteredLogs();
});
focusModeButton.addEventListener("click", () => setFocusedAgent(activeAgentName));
clearFocusButton.addEventListener("click", () => setFocusedAgent(""));
agentRoster.addEventListener("click", (event) => {
  const item = event.target.closest("[data-agent]");
  if (item) {
    setFocusedAgent(item.dataset.agent);
  }
});
window.addEventListener("resize", resizeCanvas);

resizeCanvas();
animationFrame = requestAnimationFrame(animationLoop);
loadRuns().then(replay).catch((error) => {
  inspector.innerHTML = `<h2>Spinning Wheel</h2><p>${error.message}</p>`;
});
