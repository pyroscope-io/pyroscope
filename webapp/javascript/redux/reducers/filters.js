import uniqBy from "lodash/fp/uniqBy";
import { deltaDiffImmutable } from "../../util/flamebearer";
import {
  SET_DATE_RANGE,
  SET_FROM,
  SET_UNTIL,
  SET_MAX_NODES,
  SET_LABELS,
  REFRESH,
  ADD_LABEL,
  REMOVE_LABEL,
  RECEIVE_TIMELINE,
  REQUEST_TIMELINE,
  RECEIVE_NAMES,
  REQUEST_NAMES,
  SET_LEFT_DATE_RANGE,
  SET_RIGHT_DATE_RANGE,
  SET_LEFT_FROM,
  SET_RIGHT_FROM,
  SET_LEFT_UNTIL,
  SET_RIGHT_UNTIL,
} from "../actionTypes";

const defaultName = window.initialState.appNames.find(
  (x) => x !== "pyroscope.server.cpu"
);

const initialState = {
  from: "now-1h",
  leftFrom: "now-1h",
  rightFrom: "now-30m",
  until: "now",
  leftUntil: "now-30m",
  rightUntil: "now",
  labels: [{ name: "__name__", value: defaultName || "pyroscope.server.cpu" }],
  names: window.initialState.appNames,
  timeline: null,
  isJSONLoading: false,
  maxNodes: 1024,
};

window.uniqBy = uniqBy;

function decodeTimelineData(timelineData) {
  if (!timelineData) {
    return [];
  }
  const res = [];
  let time = timelineData.startTime;
  return timelineData.samples.map((x) => {
    const res = [time * 1000, x];
    time += timelineData.durationDelta;
    return res;
  });
}

export default function (state = initialState, action) {
  switch (action.type) {
    case SET_DATE_RANGE:
      return {
        ...state,
        from: action.payload.from,
        until: action.payload.until,
      };
    case SET_FROM:
      return {
        ...state,
        from: action.payload.from,
      };
    case SET_LEFT_FROM:
      return {
        ...state,
        leftFrom: action.payload.from,
      };
    case SET_RIGHT_FROM:
      return {
        ...state,
        rightFrom: action.payload.from,
      };
    case SET_UNTIL:
      return {
        ...state,
        until: action.payload.until,
      };
    case SET_LEFT_UNTIL:
      return {
        ...state,
        leftUntil: action.payload.until,
      };
    case SET_RIGHT_UNTIL:
      return {
        ...state,
        rightUntil: action.payload.until,
      };
    case SET_LEFT_DATE_RANGE:
      return {
        ...state,
        leftFrom: action.payload.from,
        leftUntil: action.payload.until,
      };
    case SET_RIGHT_DATE_RANGE:
      return {
        ...state,
        rightFrom: action.payload.from,
        rightUntil: action.payload.until,
      };
    case SET_MAX_NODES:
      return {
        ...state,
        maxNodes: action.payload.maxNodes,
      };
    case REFRESH:
      return {
        ...state,
        refreshToken: Math.random(),
      };
    case SET_LABELS:
      return { ...state, labels: action.payload.labels };
    case ADD_LABEL:
      return {
        ...state,
        labels: uniqBy("name", [action.payload].concat(state.labels)),
      };
    case REMOVE_LABEL:
      return {
        ...state,
        labels: state.labels.filter((x) => x.name !== action.payload.name),
      };
    case REQUEST_TIMELINE:
      return {
        ...state,
        isJSONLoading: true,
      };
    case RECEIVE_TIMELINE:
      return {
        ...state,
        flamebearer: deltaDiffImmutable(action.payload.flamebearer),
        timeline: decodeTimelineData(action.payload.timeline),
        isJSONLoading: false,
      };
    case REQUEST_NAMES:
      return {
        ...state,
        areNamesLoading: true,
      };
    case RECEIVE_NAMES:
      let { labels } = state;
      const firstName = action.payload.names[0] || "none";
      if (labels.filter((x) => x.name === "__name__").length === 0) {
        labels = labels.concat([{ name: "__name__", value: firstName }]);
      }
      return {
        ...state,
        names: action.payload.names,
        areNamesLoading: false,
        labels,
      };
    default:
      return state;
  }
}
