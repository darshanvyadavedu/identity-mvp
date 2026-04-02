import { VERSION } from '../../../version.mjs';

const BASE_USER_AGENT = `ui-react-liveness/${VERSION}`;
const getLivenessUserAgent = () => {
    return BASE_USER_AGENT;
};

export { getLivenessUserAgent };
