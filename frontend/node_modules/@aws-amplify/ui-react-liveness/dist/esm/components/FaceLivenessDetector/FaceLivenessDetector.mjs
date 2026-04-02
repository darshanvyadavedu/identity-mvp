import * as React from 'react';
import { fetchAuthSession } from 'aws-amplify/auth';
import FaceLivenessDetectorCore from './FaceLivenessDetectorCore.mjs';

const credentialProvider = async () => {
    const { credentials } = await fetchAuthSession();
    if (!credentials) {
        throw new Error('No credentials provided');
    }
    return credentials;
};
function FaceLivenessDetector(props) {
    const { config, ...rest } = props;
    return (React.createElement(FaceLivenessDetectorCore, { ...rest, config: { credentialProvider, ...config } }));
}

export { FaceLivenessDetector as default };
