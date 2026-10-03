// SPDX-License-Identifier: MIT

package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/event"
)

// ImageAdmission is the image slice of run admission. Caption is retained for
// ingress adapters that archive attachments; Images contains only refs that
// the primary model can consume.
type ImageAdmission struct {
	Intent  string
	Images  []string
	Caption string
}

// AdmitImages confirms primary-model vision support, captions with the configured
// sidecar, or rejects with a correlated audit record. All image ingress adapters
// use this boundary before invoking the run engine.
func (k *Kernel) AdmitImages(ctx context.Context, corr, model, intent string, images []string) (ImageAdmission, error) {
	admission := ImageAdmission{Intent: intent, Images: images}
	if len(images) == 0 {
		return admission, nil
	}
	if strings.TrimSpace(admission.Intent) == "" {
		admission.Intent = "Describe the attached image(s)."
	}
	if model == "" {
		model = modelFromCtx(ctx)
	}
	if model == "" {
		model = k.Model()
	}
	if cat := k.Catalog(); cat != nil {
		if _, m := cat.FindModel(model); m != nil && m.SupportsVision() {
			return admission, nil
		}
	}
	caption, err := k.DescribeImages(ctx, corr, images, "")
	if err == nil && strings.TrimSpace(caption) != "" {
		admission.Caption = caption
		admission.Intent += "\n\n[Image description (analyzed by a vision model):\n" + caption + "\n]"
		admission.Images = nil
		return admission, nil
	}
	if ctx.Err() != nil {
		return admission, ctx.Err()
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       "governor.capability",
		Kind:          event.KindCapabilityRejected,
		Actor:         "runtime",
		CorrelationID: corr,
		Payload: map[string]any{
			"model":            model,
			"capability":       "vision",
			"images_requested": len(images),
		},
	})
	return admission, fmt.Errorf("model %q does not support vision (image input); add a vision-capable provider key or attach images only to a vision-capable model (see `agt provider check --caps`)", model)
}
