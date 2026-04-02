/**
 * The abstract class representing FaceDetection
 * to be implemented for different libraries.
 */
class FaceDetection {
    /**
     * Triggers the `loadModels` method and stores
     * the corresponding promise to be awaited later.
     */
    triggerModelLoading() {
        this.modelLoadingPromise = this.loadModels();
    }
}

export { FaceDetection };
